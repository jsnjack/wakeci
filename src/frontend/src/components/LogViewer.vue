<template>
    <div
        class="log-view stack gap-0"
        :style="{ '--log-height': tall ? '80vh' : height }"
        data-cy="log-viewer"
        @keydown="onKeyDown"
    >
        <div class="log-bar cluster gap-1">
            <template v-if="searchOpen">
                <label class="input-icon log-search">
                    <i class="icon small">search</i>
                    <input
                        ref="searchInput"
                        v-model="query"
                        type="search"
                        class="input"
                        aria-label="Search the log"
                        :placeholder="errors ? 'Errors: failures, exceptions, exit codes' : 'Search the log'"
                        data-cy="log-search"
                        @keydown.enter.prevent="$event.shiftKey ? previous() : next()"
                        @keydown.esc.prevent="closeSearch"
                    />
                </label>
                <span
                    class="muted text-sm nums nowrap-text"
                    data-cy="log-search-count"
                    >{{ countText }}</span
                >
                <button
                    type="button"
                    class="btn btn-ghost btn-icon btn-sm"
                    aria-label="Previous match"
                    data-tip="Previous (Shift+Enter)"
                    :disabled="!matchCount"
                    @click="previous"
                >
                    <i class="icon small">keyboard_arrow_up</i>
                </button>
                <button
                    type="button"
                    class="btn btn-ghost btn-icon btn-sm"
                    aria-label="Next match"
                    data-tip="Next (Enter)"
                    :disabled="!matchCount"
                    @click="next"
                >
                    <i class="icon small">keyboard_arrow_down</i>
                </button>
                <button
                    type="button"
                    class="btn btn-ghost btn-sm"
                    :aria-pressed="caseSensitive ? 'true' : 'false'"
                    aria-label="Match case"
                    data-tip="Match case"
                    :disabled="errors"
                    @click="caseSensitive = !caseSensitive"
                >
                    Aa
                </button>
                <label class="cluster nowrap gap-1 text-sm log-toggle">
                    <input
                        v-model="onlyMatching"
                        type="checkbox"
                        data-cy="log-only-matching"
                    />
                    Only matching lines
                </label>
            </template>
            <button
                v-else
                type="button"
                class="btn btn-ghost btn-icon btn-sm"
                aria-label="Search the log"
                data-tip="Search (Ctrl+F)"
                data-cy="log-search-open"
                @click="openSearch"
            >
                <i class="icon small">search</i>
            </button>
            <button
                type="button"
                class="btn btn-ghost btn-sm"
                :aria-pressed="errors ? 'true' : 'false'"
                data-tip="Find what failed: test failures, errors, exceptions, a failed exit"
                data-cy="log-errors"
                @click="toggleErrors"
            >
                Errors
            </button>
            <span class="grow"></span>
            <span
                v-if="dropped"
                class="muted text-sm"
                >The first {{ dropped.toLocaleString("en-US") }} lines aren't shown</span
            >
            <button
                type="button"
                class="btn btn-ghost btn-icon btn-sm"
                :aria-pressed="wrap ? 'true' : 'false'"
                aria-label="Wrap long lines"
                data-tip="Wrap long lines"
                @click="wrap = !wrap"
            >
                <i class="icon small">wrap_text</i>
            </button>
            <button
                type="button"
                class="btn btn-ghost btn-icon btn-sm"
                :aria-label="tall ? 'Smaller' : 'Taller'"
                :data-tip="tall ? 'Smaller' : 'Taller'"
                @click="tall = !tall"
            >
                <i class="icon small">{{ tall ? "unfold_less" : "unfold_more" }}</i>
            </button>
            <button
                v-if="marked"
                type="button"
                class="btn btn-ghost btn-icon btn-sm"
                aria-label="Copy the marked lines"
                data-tip="Copy the marked lines"
                data-cy="log-copy-lines"
                @click="copyMarked"
            >
                <i class="icon small">content_copy</i>
            </button>
            <button
                type="button"
                class="btn btn-ghost btn-icon btn-sm"
                aria-label="Reload the log"
                data-tip="Reload"
                data-cy="reload"
                @click="load"
            >
                <i class="icon small">sync</i>
            </button>
            <a
                class="btn btn-ghost btn-icon btn-sm"
                :href="rawURL"
                target="_blank"
                rel="noopener noreferrer"
                aria-label="Raw log"
                data-tip="Raw log"
            >
                <i class="icon small">open_in_new</i>
            </a>
        </div>
        <div
            ref="editor"
            class="log-editor"
            :class="{ loading }"
            data-cy="task-log"
        ></div>
        <button
            v-if="newLines > 0"
            type="button"
            class="btn btn-primary btn-sm log-pill"
            data-cy="log-new-lines"
            @click="scrollToEnd"
        >
            <i class="icon small">arrow_downward</i>
            {{ newLines.toLocaleString("en-US") }} new {{ newLines === 1 ? "line" : "lines"
            }}<template v-if="newMatches"> · {{ newMatches }} new {{ newMatches === 1 ? "match" : "matches" }}</template>
        </button>
    </div>
</template>

<script>
import "@/assets/log.css";
import { AnsiParser, prefixEnd } from "@/log/ansi.js";
import { backgroundColor, readableColor } from "@/log/colors.js";
import { LogSearch } from "@/log/search.js";
import { Compartment, EditorState, RangeSetBuilder, StateEffect, StateField } from "@codemirror/state";
import { Decoration, EditorView, ViewPlugin, WidgetType, lineNumbers } from "@codemirror/view";
import axios from "axios";
import vuex from "vuex";

// A log above this size opens with its last lines (tail), and a running
// task's log that grows past it drops its oldest lines
const MAX_LOG_BYTES = 50 * 1024 * 1024;
const TAIL_LINES = 10000;

const URL_RE = /\bhttps?:\/\/[^\s<>"'`\\]+/g;

// A change that makes the decorations be drawn again
const redraw = StateEffect.define();

// The lines "Only matching lines" hides, as block widgets
const setGaps = StateEffect.define();
const gapsField = StateField.define({
    create: () => Decoration.none,
    update(gaps, tr) {
        gaps = gaps.map(tr.changes);
        for (const effect of tr.effects) {
            if (effect.is(setGaps)) {
                gaps = effect.value;
            }
        }
        return gaps;
    },
    provide: (field) => EditorView.decorations.from(field),
});

class GapWidget extends WidgetType {
    constructor(lines) {
        super();
        this.lines = lines;
    }
    eq(other) {
        return other.lines === this.lines;
    }
    toDOM() {
        const el = document.createElement("div");
        el.className = "log-gap";
        el.textContent = `⋯ ${this.lines.toLocaleString("en-US")} ${this.lines === 1 ? "line" : "lines"}`;
        return el;
    }
    get estimatedHeight() {
        return 20;
    }
}

// The time prefix ("[     707ms] ") shows as a narrow column with the time
// alone, so lines have room for their text; wrapped lines go under the text
// (the theme's indent). The text, and copies of it, keep the prefix.
class TimeWidget extends WidgetType {
    constructor(time) {
        super();
        this.time = time;
    }
    eq(other) {
        return other.time === this.time;
    }
    toDOM() {
        const span = document.createElement("span");
        span.className = "log-time";
        span.textContent = this.time;
        return span;
    }
}
// A line without the prefix, such as a command's second line, starts where
// the text of the others does
const plainLine = Decoration.line({ class: "log-plain" });
const hitMark = Decoration.mark({ class: "log-hit" });
const currentHitMark = Decoration.mark({ class: "log-hit-current" });
const markedLine = Decoration.line({ class: "log-marked" });

// The class and style of a span of colour (ansi.js), in the theme
function spanDecoration(span, theme) {
    const { style } = span;
    const classes = [];
    const css = [];
    let fg = style.fg;
    let bg = style.bg;
    if (style.inverse) {
        [fg, bg] = [bg, fg === null ? 7 : fg];
    }
    if (typeof fg === "number") {
        classes.push(`ansi-fg-${fg}`);
    } else if (fg) {
        css.push(`color: ${readableColor(fg, theme)}`);
    }
    if (typeof bg === "number") {
        classes.push(`ansi-bg-${bg}`);
    } else if (bg) {
        css.push(`background-color: ${backgroundColor(bg)}`);
    }
    for (const name of ["bold", "dim", "italic", "underline"]) {
        if (style[name]) {
            classes.push(`ansi-${name}`);
        }
    }
    const attributes = css.length ? { style: css.join("; ") } : {};
    if (span.href) {
        return Decoration.mark({
            tagName: "a",
            class: ["log-link", ...classes].join(" "),
            attributes: { ...attributes, href: span.href, rel: "noopener noreferrer", target: "_blank" },
        });
    }
    return Decoration.mark({ class: classes.join(" "), attributes });
}

const linkMark = (href) => Decoration.mark({ tagName: "a", class: "log-link", attributes: { href, rel: "noopener noreferrer", target: "_blank" } });

// LogViewer shows a task's log: its colours, links and line numbers, a
// search, and, while the task runs, the lines that arrive, numbered by the
// server so none is shown twice or missed (L2). Only the lines in view are
// drawn, so it stays quick at 10 MB and more (docs/logs.md).
export default {
    props: {
        buildId: {
            type: Number,
            required: true,
        },
        task: {
            type: Object,
            required: true,
        },
        // The box's height, which shorter logs don't fill
        height: {
            type: String,
            default: "420px",
        },
        // Opens at the log's end, as for a failed task
        openAtEnd: {
            type: Boolean,
            default: false,
        },
        // Lines to mark and show, { from, to }, from the address (#t13-L120)
        mark: {
            type: Object,
            default: null,
        },
    },
    emits: ["mark"],
    data() {
        return {
            loading: true,
            searchOpen: false,
            query: "",
            // errors searches for errorMatches instead of the query
            errors: false,
            caseSensitive: false,
            onlyMatching: false,
            matchCount: 0,
            countText: "",
            wrap: true,
            tall: false,
            newLines: 0,
            newMatches: 0,
            // Lines dropped from the start: a long log's tail, or a running
            // task's log past MAX_LOG_BYTES
            dropped: 0,
            marked: null,
        };
    },
    computed: {
        ...vuex.mapState(["theme"]),
        rawURL() {
            return `/storage/build/${this.buildId}/task_${this.task.id}.log`;
        },
        running() {
            return this.task.status === "running";
        },
    },
    watch: {
        query(value) {
            // Typing searches the text instead
            if (value) {
                this.errors = false;
            }
            this.searchSoon();
        },
        errors() {
            this.search();
        },
        caseSensitive() {
            this.search();
        },
        onlyMatching() {
            this.updateGaps();
        },
        wrap(value) {
            this.view.dispatch({ effects: this.wrapping.reconfigure(value ? EditorView.lineWrapping : []) });
        },
        theme() {
            this.view.dispatch({ effects: redraw.of(null) });
        },
        mark(value) {
            this.showMark(value);
        },
    },
    mounted() {
        // Not reactive: CodeMirror keeps its own state
        this.wrapping = new Compartment();
        this.spans = new Map();
        this.parser = new AnsiParser();
        this.pending = [];
        this.searchState = null;
        this.current = -1;
        this.view = new EditorView({
            parent: this.$refs.editor,
            state: this.newState(""),
        });
        // Following the end stops when the user scrolls up and starts again
        // at the end; the page's own scrolls don't count (onScroll)
        this.following = this.openAtEnd || this.running;
        this.userScrolledDownAt = 0;
        const scroller = this.view.scrollDOM;
        scroller.addEventListener("wheel", this.onUserScroll, { passive: true });
        scroller.addEventListener("touchmove", this.onUserScroll, { passive: true });
        scroller.addEventListener("keydown", this.onUserScroll);
        scroller.addEventListener("scroll", this.onScroll, { passive: true });
        this.emitter.on(this.messageType(), this.onMessage);
        this.load();
    },
    unmounted() {
        this.emitter.off(this.messageType(), this.onMessage);
        cancelAnimationFrame(this.frame);
        clearTimeout(this.searchTimer);
        this.view.destroy();
    },
    methods: {
        messageType() {
            return `build:log:${this.buildId}:task-${this.task.id}`;
        },
        newState(doc) {
            const viewer = this;
            const decorations = ViewPlugin.fromClass(
                class {
                    constructor(view) {
                        this.decorations = viewer.decorate(view);
                    }
                    update(update) {
                        if (update.docChanged || update.viewportChanged || update.transactions.some((tr) => tr.effects.some((e) => e.is(redraw)))) {
                            this.decorations = viewer.decorate(update.view);
                        }
                    }
                },
                { decorations: (plugin) => plugin.decorations },
            );
            return EditorState.create({
                doc,
                extensions: [
                    EditorState.readOnly.of(true),
                    lineNumbers({
                        formatNumber: (n) => String(n + this.dropped),
                        domEventHandlers: {
                            mousedown: (view, line, event) => {
                                this.markLine(view.state.doc.lineAt(line.from).number + this.dropped, event.shiftKey);
                                return true;
                            },
                        },
                    }),
                    this.wrapping.of(this.wrap ? EditorView.lineWrapping : []),
                    decorations,
                    gapsField,
                    EditorView.domEventHandlers({
                        // Links open in a new tab; CodeMirror would only move
                        // the cursor
                        click: (event) => {
                            const link = event.target.closest("a.log-link");
                            if (link) {
                                window.open(link.href, "_blank", "noopener,noreferrer");
                                event.preventDefault();
                                return true;
                            }
                            return false;
                        },
                    }),
                    EditorView.theme({
                        "&": { backgroundColor: "var(--log-bg)", color: "var(--log-fg)", fontSize: "13px" },
                        "&.cm-focused": { outline: "none" },
                        ".cm-scroller": { fontFamily: "var(--font-mono)", lineHeight: "1.5", maxHeight: "var(--log-height, 420px)" },
                        ".cm-gutters": { backgroundColor: "var(--log-bg)", color: "var(--log-muted)", border: "none" },
                        ".cm-lineNumbers .cm-gutterElement": { padding: "0 10px 0 12px", cursor: "pointer", userSelect: "none" },
                        ".cm-lineNumbers .cm-gutterElement:hover": { color: "var(--primary)", textDecoration: "underline" },
                        ".cm-content": { padding: "6px 0" },
                        // Room for the time column (log-time), which a wrapped line's
                        // second row leaves empty
                        ".cm-line": { padding: "0 12px 0 calc(var(--log-time) + 2px)", textIndent: "calc(-1 * var(--log-time))" },
                        ".cm-line.log-plain": { textIndent: "0" },
                        ".log-time": {
                            display: "inline-block",
                            inlineSize: "var(--log-time)",
                            paddingInlineEnd: "1ch",
                            textAlign: "end",
                            textIndent: "0",
                            color: "var(--log-muted)",
                            userSelect: "none",
                        },
                        "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": { backgroundColor: "rgb(103 80 164 / 0.3)" },
                    }),
                ],
            });
        },
        // decorate draws the lines in view: the time prefix in grey, the
        // colours and links, the search's matches and the marked lines
        decorate(view) {
            const builder = new RangeSetBuilder();
            const doc = view.state.doc;
            const current = this.searchState?.matches[this.current];
            for (const { from, to } of view.visibleRanges) {
                for (let pos = from; pos <= to;) {
                    const line = doc.lineAt(pos);
                    const number = line.number + this.dropped;
                    if (this.marked && number >= this.marked.from && number <= this.marked.to) {
                        builder.add(line.from, line.from, markedLine);
                    }
                    const marks = [];
                    const end = prefixEnd(line.text);
                    if (end > 0) {
                        marks.push([0, end, Decoration.replace({ widget: new TimeWidget(line.text.slice(1, end - 2).trim()) })]);
                    } else {
                        builder.add(line.from, line.from, plainLine);
                    }
                    const spans = this.spans.get(line.number) || [];
                    for (const span of spans) {
                        if (span.to > end) {
                            marks.push([Math.max(span.from, end), span.to, spanDecoration(span, this.theme)]);
                        }
                    }
                    if (!spans.some((span) => span.href)) {
                        for (const m of line.text.matchAll(URL_RE)) {
                            const href = m[0].replace(/[.,;:)\]]+$/, "");
                            marks.push([m.index, m.index + href.length, linkMark(href)]);
                        }
                    }
                    if (this.searchState?.active) {
                        for (const [hitFrom, hitTo] of this.searchState.find(line.text)) {
                            const isCurrent = current && current.line === line.number && current.from === hitFrom;
                            marks.push([hitFrom, hitTo, isCurrent ? currentHitMark : hitMark]);
                        }
                    }
                    marks.sort((a, b) => a[0] - b[0] || a[1] - b[1]);
                    for (const [markFrom, markTo, deco] of marks) {
                        if (markTo > markFrom) {
                            builder.add(line.from + markFrom, line.from + markTo, deco);
                        }
                    }
                    pos = line.to + 1;
                }
            }
            return builder.finish();
        },
        // load reads the log: whole, or its last lines when it is long, then
        // the lines that arrived meanwhile
        load() {
            this.loading = true;
            this.pending = [];
            // The log read has them; appended after it, they'd show twice
            cancelAnimationFrame(this.frame);
            this.queued = [];
            const size = this.task.logSize || 0;
            const tail = size > MAX_LOG_BYTES;
            const url = `/api/build/${this.buildId}/log/${this.task.id}?${tail ? `tail=${TAIL_LINES}` : "ansi=1"}`;
            axios
                .get(url, { responseType: "text", transformResponse: (data) => data })
                .then((response) => this.setLog(response.data, tail, Number(response.headers["x-first-line"]) || 0))
                .catch(() => this.setLog("", false));
        },
        // first is the number of a tail's first line in the log (X-First-Line)
        setLog(text, tail, first = 0) {
            this.parser = new AnsiParser();
            this.spans = new Map();
            const lines = text === "" ? [] : text.replace(/\n$/, "").split("\n");
            // A running task may be writing its last line; the live log
            // sends it whole, so the line count leaves it out (a tail has
            // none: the server leaves it out)
            if (!tail && text !== "" && !text.endsWith("\n")) {
                lines.pop();
            }
            const visible = lines.map((line, i) => this.parse(line, i + 1));
            // A tail is numbered from where it starts, so line links,
            // matches of the search of all tasks and the streamed lines'
            // check stay right; one that doesn't say takes streamed lines
            // without the check
            this.tail = tail && !first;
            this.dropped = first > 0 ? first - 1 : 0;
            // Read again because live lines skipped some (gapID): if the
            // file still doesn't have them, as when the disk is full, reading
            // it again won't help, so live lines are taken as they come
            if (this.gapID && this.dropped + lines.length + 1 < this.gapID) {
                this.tail = true;
            }
            this.gapID = 0;
            this.lineCount = lines.length;
            // Without the last line break, which would show an empty line
            this.view.setState(this.newState(visible.join("\n")));
            this.loading = false;
            this.newLines = 0;
            this.newMatches = 0;
            this.search();
            const pending = this.pending;
            this.pending = [];
            pending.forEach((msg) => this.onMessage(msg));
            this.$nextTick(() => {
                if (this.mark) {
                    this.following = false;
                    this.showMark(this.mark);
                } else if (this.following) {
                    this.scrollToEnd();
                }
            });
        },
        // parse returns a line's text and keeps its colours, for line number
        parse(line, number) {
            const { text, spans } = this.parser.line(line);
            if (spans.length) {
                this.spans.set(number, spans);
            } else {
                this.spans.delete(number);
            }
            return text;
        },
        // onMessage takes a batch of lines the server sent, numbered from
        // msg.id: those the log already has are dropped, and a gap, such as
        // after a reconnect, reads the log again
        onMessage(msg) {
            if (this.loading) {
                this.pending.push(msg);
                return;
            }
            const lines = (msg.ansi || msg.data).replace(/\n$/, "").split("\n");
            // With the lines that wait for the next frame (appendQueued):
            // two batches in one frame, or in a background tab, are no gap
            const have = this.dropped + this.lineCount + (this.queued?.length || 0);
            let start = 0;
            if (!this.tail) {
                if (msg.id > have + 1) {
                    this.gapID = msg.id;
                    this.load();
                    // Replayed after the read, which usually has its lines
                    // already: then they are skipped
                    this.pending.push(msg);
                    return;
                }
                start = have + 1 - msg.id;
                if (start >= lines.length) {
                    return;
                }
            }
            this.queued = (this.queued || []).concat(lines.slice(start));
            cancelAnimationFrame(this.frame);
            this.frame = requestAnimationFrame(this.appendQueued);
        },
        // appendQueued adds the lines that arrived, once a frame, and follows
        // the end only while it is in view
        appendQueued() {
            const lines = this.queued || [];
            this.queued = [];
            if (lines.length === 0) {
                return;
            }
            const first = this.lineCount + 1;
            const visible = lines.map((line, i) => this.parse(line, first + i));
            const insert = (this.lineCount > 0 ? "\n" : "") + visible.join("\n");
            this.view.dispatch({ changes: { from: this.view.state.doc.length, insert } });
            this.lineCount += lines.length;
            let added = 0;
            if (this.searchState?.active) {
                const before = this.searchState.count;
                this.searchState.add(first, visible);
                added = this.searchState.count - before;
                this.showCount();
                this.updateGaps();
            }
            this.dropOldLines();
            if (this.following) {
                this.scrollToEnd();
            } else {
                this.newLines += lines.length;
                this.newMatches += added;
            }
        },
        // onUserScroll: scrolling up means reading, so following stops at once.
        // Only a scroll down by the user can start it again (onScroll): a
        // scroll to the end the viewer asked for before can land after it.
        onUserScroll(event) {
            const up = event.type === "wheel" ? event.deltaY < 0 : event.type === "touchmove" || ["ArrowUp", "PageUp", "Home"].includes(event.key);
            const down = event.type === "wheel" ? event.deltaY > 0 : ["ArrowDown", "PageDown", "End", " "].includes(event.key);
            if (up) {
                this.following = false;
                this.userScrolledDownAt = 0;
            } else if (down) {
                this.userScrolledDownAt = performance.now();
            }
        },
        // onScroll: a user who scrolls back down to the end follows it again
        onScroll() {
            if (!this.following && performance.now() - this.userScrolledDownAt < 1000 && this.isAtEnd()) {
                this.following = true;
                this.newLines = 0;
                this.newMatches = 0;
            }
        },
        // dropOldLines keeps a running task's log under MAX_LOG_BYTES
        dropOldLines() {
            const doc = this.view.state.doc;
            if (doc.length <= MAX_LOG_BYTES) {
                return;
            }
            const cut = doc.lineAt(Math.floor(doc.length / 5));
            const removed = cut.number;
            this.view.dispatch({ changes: { from: 0, to: Math.min(doc.length, cut.to + 1) } });
            const spans = new Map();
            for (const [line, value] of this.spans) {
                if (line > removed) {
                    spans.set(line - removed, value);
                }
            }
            this.spans = spans;
            this.dropped += removed;
            this.lineCount -= removed;
            this.search();
        },
        isAtEnd() {
            const scroller = this.view.scrollDOM;
            return scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 8;
        },
        scrollToEnd() {
            this.following = true;
            this.newLines = 0;
            this.newMatches = 0;
            // Only the viewer scrolls: scrollIntoView would also move the
            // page, e.g. a phone's down to the end of a failed log, hiding
            // the build's title. Again once CodeMirror has measured the lines
            const scroller = this.view.scrollDOM;
            scroller.scrollTop = scroller.scrollHeight;
            this.view.requestMeasure({
                read: () => scroller.scrollHeight,
                write: (height) => {
                    scroller.scrollTop = height;
                },
            });
        },
        openSearch() {
            this.searchOpen = true;
            this.$nextTick(() => this.$refs.searchInput?.focus());
        },
        closeSearch() {
            this.searchOpen = false;
            this.query = "";
            this.errors = false;
            this.onlyMatching = false;
        },
        // toggleErrors turns the search into one for what failed, or back
        toggleErrors() {
            this.errors = !this.errors;
            if (this.errors) {
                this.query = "";
                this.openSearch();
            }
        },
        // Ctrl+F and / inside the viewer search it: the browser can't find
        // lines that aren't drawn
        onKeyDown(event) {
            const inInput = event.target.closest("input");
            if (((event.ctrlKey || event.metaKey) && event.key === "f") || (event.key === "/" && !inInput)) {
                event.preventDefault();
                this.openSearch();
            }
        },
        searchSoon() {
            clearTimeout(this.searchTimer);
            this.searchTimer = setTimeout(this.search, 150);
        },
        // search finds the query in the whole log; lines that arrive later
        // add their matches (appendQueued)
        search() {
            if (!this.view) {
                return;
            }
            const state = new LogSearch(this.query, this.caseSensitive, this.errors);
            if (state.active) {
                const lines = [];
                for (const iter = this.view.state.doc.iterLines(); !iter.next().done;) {
                    lines.push(iter.value);
                }
                state.add(1, lines);
            }
            this.searchState = state;
            this.current = state.matches.length ? state.nearest(this.topLine()) : -1;
            this.showCount();
            this.updateGaps();
            this.view.dispatch({ effects: redraw.of(null) });
        },
        showCount() {
            const state = this.searchState;
            this.matchCount = state?.count || 0;
            if (!state?.active) {
                this.countText = "";
            } else if (this.onlyMatching) {
                this.countText = `${state.lines.length.toLocaleString("en-US")} matching lines of ${this.lineCount.toLocaleString("en-US")}`;
            } else if (state.count === 0) {
                this.countText = state.errors ? "No errors found" : "No matches";
            } else {
                this.countText = `${(this.current + 1).toLocaleString("en-US")} of ${state.countText()}`;
            }
        },
        topLine() {
            const block = this.view.lineBlockAtHeight(this.view.scrollDOM.scrollTop);
            return this.view.state.doc.lineAt(block.from).number;
        },
        next() {
            this.go(1);
        },
        previous() {
            this.go(-1);
        },
        // go moves to the next or previous match; the view leaves the end,
        // so following stops while the user reads a match
        go(step) {
            const matches = this.searchState?.matches || [];
            if (!matches.length) {
                return;
            }
            this.current = (this.current + step + matches.length) % matches.length;
            this.following = false;
            const match = matches[this.current];
            const line = this.view.state.doc.line(match.line);
            this.view.dispatch({ effects: [EditorView.scrollIntoView(line.from + match.from, { y: "center" }), redraw.of(null)] });
            this.showCount();
        },
        // updateGaps hides the lines without a match, in "Only matching lines"
        updateGaps() {
            const doc = this.view.state.doc;
            const builder = new RangeSetBuilder();
            if (this.onlyMatching && this.searchState?.active) {
                let next = 1;
                const hide = (fromLine, toLine) => {
                    if (toLine >= fromLine) {
                        builder.add(
                            doc.line(fromLine).from,
                            doc.line(toLine).to,
                            Decoration.replace({ widget: new GapWidget(toLine - fromLine + 1), block: true }),
                        );
                    }
                };
                for (const line of this.searchState.lines) {
                    hide(next, line - 1);
                    next = line + 1;
                }
                hide(next, this.lineCount);
            }
            this.view.dispatch({ effects: setGaps.of(builder.finish()) });
            this.showCount();
        },
        // markLine marks a line, or the range to it with Shift, and puts it
        // in the address
        markLine(number, extend) {
            const from = extend && this.marked ? Math.min(this.marked.from, number) : number;
            const to = extend && this.marked ? Math.max(this.marked.from, number) : number;
            this.marked = { from, to };
            this.view.dispatch({ effects: redraw.of(null) });
            this.$emit("mark", this.marked);
        },
        showMark(mark) {
            if (!mark || !this.view || this.loading) {
                return;
            }
            this.marked = { from: mark.from, to: mark.to };
            this.following = false;
            const line = mark.from - this.dropped;
            if (line >= 1 && line <= this.view.state.doc.lines) {
                this.view.dispatch({ effects: [EditorView.scrollIntoView(this.view.state.doc.line(line).from, { y: "center" }), redraw.of(null)] });
            }
        },
        copyMarked() {
            const doc = this.view.state.doc;
            const from = doc.line(Math.max(1, this.marked.from - this.dropped)).from;
            const to = doc.line(Math.min(doc.lines, this.marked.to - this.dropped)).to;
            navigator.clipboard.writeText(doc.sliceString(from, to)).then(
                () => this.$notify({ text: "Copied", type: "primary", duration: 1500 }),
                () => {},
            );
        },
    },
};
</script>

<style scoped>
/* Nothing here clips its overflow, or the buttons' tips would be cut off:
   the bar and the editor round their own corners, and the bar wraps on a
   narrow screen instead of scrolling */
.log-view {
    /* The time column: 7 characters for times such as 13m33.1s, and a space */
    --log-time: 8ch;
    position: relative;
    container-type: inline-size;
    border: 1px solid var(--outline-variant);
    border-radius: var(--radius-control);
    background-color: var(--log-bg);
}

.log-bar {
    padding: var(--space-1) var(--space-2);
    border-block-end: 1px solid var(--outline-variant);
    border-start-start-radius: inherit;
    border-start-end-radius: inherit;
    background-color: var(--surface-container-low);
}

.log-view :deep(.cm-editor) {
    border-end-start-radius: var(--radius-control);
    border-end-end-radius: var(--radius-control);
}

.log-search {
    flex: 1 1 14rem;
    max-inline-size: 22rem;
}

.log-search .input {
    block-size: 28px;
    font-size: var(--text-sm);
}

.log-toggle {
    white-space: nowrap;
    cursor: pointer;
}

.log-editor.loading {
    min-block-size: 3rem;
    opacity: 0.6;
}

.log-pill {
    position: absolute;
    inset-block-end: var(--space-3);
    inset-inline-start: 50%;
    translate: -50% 0;
    z-index: 5;
    border-radius: var(--radius-pill);
    box-shadow: 0 4px 16px rgb(0 0 0 / 0.2);
}

/* A narrow viewer, e.g. on a phone, gives its width to the text: no time
   column (the raw log and copied lines keep the times), slimmer numbers */
@container (max-width: 34rem) {
    .log-view :deep(.cm-editor) {
        --log-time: 0px;
    }
    .log-view :deep(.log-time) {
        display: none;
    }
    .log-view :deep(.cm-lineNumbers .cm-gutterElement) {
        padding: 0 6px 0 6px;
    }
}
</style>

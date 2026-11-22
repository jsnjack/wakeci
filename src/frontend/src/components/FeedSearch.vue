<template>
    <div class="stack gap-2">
        <div class="cluster nowrap gap-1">
            <div class="search-box grow">
                <label class="input-icon">
                    <i class="icon">search</i>
                    <input
                        ref="input"
                        type="search"
                        class="input"
                        data-cy="filter"
                        aria-label="Search builds"
                        placeholder="Search by id, job, status, branch=main…"
                        role="combobox"
                        aria-autocomplete="list"
                        :aria-expanded="showSuggestions ? 'true' : 'false'"
                        aria-controls="feed-suggestions"
                        :aria-activedescendant="showSuggestions && active >= 0 ? `feed-suggestion-${active}` : null"
                        :value="modelValue"
                        @input="onInput"
                        @keydown="onKeyDown"
                        @focus="suggestOpen = true"
                        @blur="suggestOpen = false"
                    />
                    <span
                        v-show="fetching"
                        class="search-spinner status-ring"
                        aria-hidden="true"
                    ></span>
                </label>
                <ul
                    v-if="showSuggestions"
                    id="feed-suggestions"
                    class="menu-list suggestions"
                    role="listbox"
                    data-cy="search-suggestions"
                >
                    <li
                        v-for="(item, i) in suggestions"
                        :id="`feed-suggestion-${i}`"
                        :key="item.text"
                        role="option"
                        :aria-selected="i === active ? 'true' : 'false'"
                        @mousedown.prevent="accept(item)"
                    >
                        <span class="mono">{{ item.text }}</span>
                        <span class="muted text-sm">{{ item.hint }}</span>
                    </li>
                </ul>
            </div>
            <button
                type="button"
                class="btn btn-ghost btn-icon"
                aria-label="Clear the search"
                data-tip="Clear the search"
                :disabled="modelValue === ''"
                @click="$emit('clear')"
            >
                <i class="icon">backspace</i>
            </button>
            <button
                type="button"
                class="btn btn-ghost btn-icon tip-end"
                aria-label="Search examples"
                data-tip="Search examples"
                @click="helpOpen = true"
            >
                <i class="icon">help</i>
            </button>
        </div>

        <div
            class="cluster gap-1"
            data-cy="quick-filters"
        >
            <button
                type="button"
                class="btn btn-sm btn-outline btn-ghost"
                :aria-pressed="has(FAILED) ? 'true' : 'false'"
                data-cy="filter-failed"
                @click="toggle(FAILED)"
            >
                Failed
            </button>
            <button
                type="button"
                class="btn btn-sm btn-outline btn-ghost"
                :aria-pressed="has(ACTIVE) ? 'true' : 'false'"
                data-cy="filter-active"
                @click="toggle(ACTIVE)"
            >
                Running and queued
            </button>
            <button
                v-if="scheduledTerms.length"
                type="button"
                class="btn btn-sm btn-outline btn-ghost"
                :aria-pressed="has(scheduledTerms) ? 'true' : 'false'"
                data-cy="filter-scheduled"
                @click="toggle(scheduledTerms)"
            >
                Hide scheduled
            </button>
            <UiMenu
                v-if="jobNames.length"
                label="Jobs"
                icon=""
                :close-on-click="false"
                :button-class="`btn btn-sm btn-outline btn-ghost${pickedJobs.length ? ' picked' : ''}`"
                cy="filter-jobs"
            >
                <template #button>
                    {{ pickedJobs.length ? `${pickedJobs.length} ${pickedJobs.length === 1 ? "job" : "jobs"}` : "Jobs" }}
                    <i class="icon small">arrow_drop_down</i>
                </template>
                <label
                    v-for="name in jobNames"
                    :key="name"
                    role="menuitemcheckbox"
                    :aria-checked="pickedJobs.includes(name) ? 'true' : 'false'"
                    class="job-choice"
                >
                    <input
                        type="checkbox"
                        :checked="pickedJobs.includes(name)"
                        @change="toggle([`name=${name}`])"
                    />
                    {{ name }}
                </label>
            </UiMenu>
            <span
                v-if="saved.length"
                class="filter-sep"
                aria-hidden="true"
            ></span>
            <span
                v-for="(item, i) in saved"
                :key="item.name"
                class="saved"
            >
                <button
                    type="button"
                    class="btn btn-sm btn-outline btn-ghost"
                    :aria-pressed="modelValue.trim() === item.q ? 'true' : 'false'"
                    :data-tip="item.q"
                    data-cy="saved-search"
                    @click="$emit('update:modelValue', item.q)"
                >
                    {{ item.name }}
                </button>
                <button
                    type="button"
                    class="btn btn-sm btn-ghost btn-icon"
                    :aria-label="`Forget the search ${item.name}`"
                    data-cy="forget-search"
                    @click="forget(i)"
                >
                    <i class="icon small">close</i>
                </button>
            </span>
            <button
                type="button"
                class="btn btn-sm btn-ghost"
                data-cy="save-search"
                :disabled="modelValue.trim() === ''"
                @click="openSave"
            >
                <i class="icon small">bookmark_add</i> Save this search
            </button>
            <button
                type="button"
                class="btn btn-sm btn-ghost"
                data-cy="watch-search"
                :aria-pressed="searchWatched ? 'true' : 'false'"
                :disabled="query === ''"
                @click="toggleWatch"
            >
                <i class="icon small">{{ searchWatched ? "notifications_active" : "notifications_none" }}</i>
                {{ searchWatched ? "Notifying when they end" : "Notify me when they end" }}
            </button>
        </div>

        <div
            v-if="terms.length"
            class="cluster gap-1 text-sm"
            data-cy="search-terms"
        >
            <span
                v-for="(term, i) in terms"
                :key="term + i"
                class="term"
            >
                <span class="mono">{{ term }}</span>
                <button
                    type="button"
                    class="term-remove"
                    :aria-label="`Remove ${term}`"
                    data-cy="remove-term"
                    @click="removeTerm(i)"
                >
                    <i class="icon small">close</i>
                </button>
            </span>
        </div>

        <UiDialog
            v-model:open="helpOpen"
            title="Search examples"
        >
            <ul class="search-examples stack gap-2">
                <li><code>status:failed</code> Failed builds</li>
                <li><code>status:failed "status:timed out"</code> Failed or timed out builds</li>
                <li><code>+status:failed -name:test_</code> Failed builds, excluding builds with names containing "test_"</li>
                <li><code>env=prod</code> Builds whose parameter "ENV" (or label "ENV_LABEL") is exactly "prod"</li>
                <li><code>+status:failed env=prod env=uat</code> Failed builds with "ENV" equal to "prod" or "uat"</li>
                <li><code>env:prod</code> "env:prod" anywhere, so also "env:production"</li>
            </ul>
            <p class="muted text-sm">A click on a value in a row searches for it; Ctrl+click adds it to the search.</p>
            <template #actions>
                <button
                    type="button"
                    class="btn"
                    @click="helpOpen = false"
                >
                    Close
                </button>
            </template>
        </UiDialog>

        <UiDialog
            v-model:open="saveOpen"
            title="Save this search"
        >
            <form
                id="save-search-form"
                class="stack gap-2"
                @submit.prevent="save"
            >
                <label class="form-field">
                    <span class="label-text">Name</span>
                    <input
                        ref="saveName"
                        v-model="saveName"
                        class="input"
                        data-cy="save-search-name"
                        required
                    />
                </label>
                <p class="hint">
                    <span class="mono">{{ modelValue.trim() }}</span> becomes a filter in this browser. Saved as Mine, a search such as
                    <span class="mono">+author=alice</span> shows your builds, also those another job or a shared key started.
                </p>
            </form>
            <template #actions>
                <button
                    type="button"
                    class="btn btn-ghost"
                    @click="saveOpen = false"
                >
                    Cancel
                </button>
                <button
                    type="submit"
                    form="save-search-form"
                    class="btn btn-primary"
                    data-cy="save-search-confirm"
                >
                    Save
                </button>
            </template>
        </UiDialog>
    </div>
</template>

<script>
import UiDialog from "@/components/ui/UiDialog.vue";
import UiMenu from "@/components/ui/UiMenu.vue";
import { DONE_STATUSES, hasTerm, quoteTerm, splitTerms, toggleTerms } from "@/feed.js";
import { askNotificationPermission, shownName } from "@/store/utils.js";

const FAILED = ["+status=failed"];
const ACTIVE = DONE_STATUSES.map((status) => `-status=${status}`);
const STATUSES = ["pending", "running", ...DONE_STATUSES];
const SAVED_KEY = "savedSearches";
const MAX_SUGGESTIONS = 8;

function loadSaved() {
    try {
        const saved = JSON.parse(window.localStorage.getItem(SAVED_KEY) || "[]");
        return Array.isArray(saved) ? saved.filter((item) => item && typeof item.name === "string" && typeof item.q === "string") : [];
    } catch {
        return [];
    }
}

// The feed's search: the box, which suggests names and values while typing
// (F8), quick filters and saved searches that write the search (F4), and
// its terms, which can be removed one by one
export default {
    components: { UiDialog, UiMenu },
    props: {
        modelValue: {
            type: String,
            required: true,
        },
        // Each job's { defaults, labels, interval }
        jobs: {
            type: Map,
            required: true,
        },
        // The builds the feed shows, whose values the box suggests
        builds: {
            type: Array,
            default: () => [],
        },
        fetching: {
            type: Boolean,
            default: false,
        },
    },
    emits: ["update:modelValue", "clear"],
    data() {
        return {
            FAILED,
            ACTIVE,
            helpOpen: false,
            saveOpen: false,
            saveName: "",
            saved: loadSaved(),
            suggestOpen: false,
            caret: 0,
            active: -1,
        };
    },
    computed: {
        query() {
            return this.modelValue.trim();
        },
        // Notifications for the builds that match the search (L5)
        searchWatched() {
            return this.query !== "" && this.$store.state.watches.searches.includes(this.query);
        },
        jobNames() {
            return [...this.jobs.keys()].sort((a, b) => a.localeCompare(b, undefined, { numeric: true }));
        },
        scheduledTerms() {
            return this.jobNames.filter((name) => this.jobs.get(name).interval).map((name) => `-name=${name}`);
        },
        pickedJobs() {
            return this.jobNames.filter((name) => hasTerm(this.modelValue, `name=${name}`));
        },
        terms() {
            return splitTerms(this.modelValue);
        },
        // The term the caret is in, as { start, sign, name, value }; value
        // is null before an =
        token() {
            const before = this.modelValue.slice(0, this.caret);
            const start = before.lastIndexOf(" ") + 1;
            const text = before.slice(start);
            const sign = /^[+-]/.test(text) ? text[0] : "";
            const body = text.slice(sign.length).replace(/^["']/, "");
            const eq = body.indexOf("=");
            return { start, sign, name: eq < 0 ? body : body.slice(0, eq), value: eq < 0 ? null : body.slice(eq + 1) };
        },
        // Names first, the labels' before others; then, after an =, the
        // values seen in the builds the feed shows
        suggestions() {
            const { name, value } = this.token;
            if (value === null) {
                if (name === "") {
                    return [];
                }
                const labels = new Set([...this.jobs.values()].flatMap((job) => job.labels.map(shownName)));
                const params = new Set(this.builds.flatMap((build) => (build.params || []).map((param) => shownName(Object.keys(param)[0]))));
                const names = [...labels, "status", "name", "id", ...[...params].filter((n) => !labels.has(n))];
                return [...new Set(names)]
                    .filter((n) => n.startsWith(name.toLowerCase()) && n !== name.toLowerCase())
                    .slice(0, MAX_SUGGESTIONS)
                    .map((n) => ({ text: `${n}=`, hint: labels.has(n) ? "label" : ["status", "name", "id"].includes(n) ? "" : "parameter", name: true }));
            }
            const key = name.toLowerCase();
            let values;
            if (key === "status") {
                values = STATUSES;
            } else if (key === "name") {
                values = this.jobNames;
            } else {
                values = this.builds
                    .flatMap((build) => (build.params || []).map((param) => Object.entries(param)[0]).filter(([n, v]) => shownName(n) === key && v !== ""))
                    .map(([, v]) => v);
            }
            const part = value.toLowerCase();
            return [...new Set(values)]
                .filter((v) => v.toLowerCase().includes(part) && v !== value)
                .slice(0, MAX_SUGGESTIONS)
                .map((v) => ({ text: `${name}=${v}`, hint: "" }));
        },
        showSuggestions() {
            return this.suggestOpen && this.suggestions.length > 0;
        },
    },
    watch: {
        suggestions() {
            this.active = -1;
        },
    },
    methods: {
        toggleWatch() {
            if (this.searchWatched) {
                this.$store.commit("UNWATCH_SEARCH", this.query);
                return;
            }
            this.$store.commit("WATCH_SEARCH", this.query);
            askNotificationPermission();
        },
        has(terms) {
            return terms.length > 0 && terms.every((term) => hasTerm(this.modelValue, term));
        },
        toggle(terms) {
            this.$emit("update:modelValue", toggleTerms(this.modelValue, terms));
        },
        onInput(event) {
            this.caret = event.target.selectionStart;
            this.suggestOpen = true;
            this.$emit("update:modelValue", event.target.value);
        },
        onKeyDown(event) {
            if (!this.showSuggestions) {
                return;
            }
            if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                event.preventDefault();
                const n = this.suggestions.length;
                this.active = (this.active + (event.key === "ArrowDown" ? 1 : -1) + n + 1) % (n + 1);
                if (this.active === n) {
                    this.active = -1;
                }
            } else if ((event.key === "Enter" || event.key === "Tab") && this.active >= 0) {
                event.preventDefault();
                this.accept(this.suggestions[this.active]);
            } else if (event.key === "Escape") {
                this.suggestOpen = false;
            }
        },
        // accept puts a suggestion in place of the term being typed: after a
        // name the caret waits for its value
        accept(item) {
            const { start, sign } = this.token;
            const end = this.modelValue.indexOf(" ", this.caret);
            const after = end < 0 ? "" : this.modelValue.slice(end);
            const term = item.name ? sign + item.text : quoteTerm(sign + item.text);
            const value = this.modelValue.slice(0, start) + term + (item.name ? "" : after || " ");
            this.$emit("update:modelValue", value);
            this.caret = start + term.length + (item.name ? 0 : 1);
            this.$nextTick(() => {
                this.$refs.input.focus();
                this.$refs.input.setSelectionRange(this.caret, this.caret);
            });
        },
        removeTerm(i) {
            const terms = [...this.terms];
            terms.splice(i, 1);
            this.$emit("update:modelValue", terms.join(" "));
        },
        openSave() {
            this.saveName = "";
            this.saveOpen = true;
            this.$nextTick(() => this.$refs.saveName?.focus());
        },
        save() {
            const name = this.saveName.trim();
            if (!name) {
                return;
            }
            this.saved = [...this.saved.filter((item) => item.name !== name), { name, q: this.modelValue.trim() }];
            this.store();
            this.saveOpen = false;
        },
        forget(i) {
            this.saved = this.saved.filter((_, j) => j !== i);
            this.store();
        },
        store() {
            try {
                window.localStorage.setItem(SAVED_KEY, JSON.stringify(this.saved));
            } catch {
                // A browser that keeps nothing forgets them on reload
            }
        },
    },
};
</script>

<style scoped>
.search-box {
    position: relative;
}

.search-box input[type="search"] {
    padding-inline-end: 34px;
}

.search-spinner {
    position: absolute;
    inset-inline-end: 10px;
    inline-size: 16px;
    block-size: 16px;
    --_c: var(--primary);
}

.suggestions {
    inset-inline: 0 auto;
    min-inline-size: 16rem;
    margin: 0;
    list-style: none;
}

.suggestions li {
    justify-content: space-between;
}

.suggestions li[aria-selected="true"] {
    background-color: var(--secondary-container);
}

.filter-sep {
    inline-size: 1px;
    block-size: 20px;
    margin-inline: var(--space-1);
    background-color: var(--outline-variant);
}

.saved {
    display: inline-flex;
    align-items: center;
}

.job-choice {
    cursor: pointer;
}

.term {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    padding: 0 2px 0 8px;
    border-radius: var(--radius-control);
    background-color: var(--surface-container-high);
}

.term-remove {
    display: inline-flex;
    padding: 2px;
    border: none;
    border-radius: 4px;
    background: none;
    color: var(--on-surface-variant);
    cursor: pointer;
}

.term-remove:hover {
    background-color: color-mix(in srgb, var(--on-surface) 10%, transparent);
}

.search-examples {
    padding-inline-start: 0;
    list-style: none;
}

.search-examples code {
    margin-inline-end: var(--space-2);
    padding: 1px 6px;
    border-radius: 4px;
    background-color: var(--surface-container-highest);
}
</style>

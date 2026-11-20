// Colour codes (SGR) and terminal hyperlinks (OSC 8) in a task's colour log
// (task_N.ansi.log), parsed into the visible text and spans of style. The
// viewer shows the text and draws the spans of the lines in view only.

// The sequences the server keeps in the colour log (colors.go): escape
// sequences start with control characters
// eslint-disable-next-line no-control-regex
const SEQUENCE = /\x1b\[([0-9;:]*)m|\x1b\]8;[^;\x07\x1b]*;([^\x07\x1b]*)(?:\x07|\x1b\\)/g;

// prefixEnd returns where a line's text starts, after wakeci's time prefix
// ("[    1.200s] "), as the server's grep does (logText)
export function prefixEnd(line) {
    if (line.startsWith("[")) {
        const end = line.indexOf("] ");
        if (end !== -1) {
            return end + 2;
        }
    }
    return 0;
}

// The colours of the 256-colour table above the 16 basic ones, as [r, g, b]
export function color256(n) {
    if (n < 16) {
        return null;
    }
    if (n >= 232) {
        const v = 8 + (n - 232) * 10;
        return [v, v, v];
    }
    const i = n - 16;
    const level = (x) => (x === 0 ? 0 : 55 + x * 40);
    return [level(Math.floor(i / 36)), level(Math.floor(i / 6) % 6), level(i % 6)];
}

// A style: fg and bg are a basic colour 0-15, [r, g, b], or null
function plainStyle() {
    return { fg: null, bg: null, bold: false, dim: false, italic: false, underline: false, inverse: false };
}

function isPlain(style) {
    return style.fg === null && style.bg === null && !style.bold && !style.dim && !style.italic && !style.underline && !style.inverse;
}

// applySGR changes style by the parameters of one SGR code, e.g. "1;31"
function applySGR(style, params) {
    const codes =
        params === ""
            ? [0]
            : params
                  .replaceAll(":", ";")
                  .split(";")
                  .map((p) => (p === "" ? 0 : Number(p)));
    for (let i = 0; i < codes.length; i++) {
        const c = codes[i];
        if (c === 0) {
            Object.assign(style, plainStyle());
        } else if (c === 1) {
            style.bold = true;
        } else if (c === 2) {
            style.dim = true;
        } else if (c === 3) {
            style.italic = true;
        } else if (c === 4) {
            style.underline = true;
        } else if (c === 7) {
            style.inverse = true;
        } else if (c === 22) {
            style.bold = style.dim = false;
        } else if (c === 23) {
            style.italic = false;
        } else if (c === 24) {
            style.underline = false;
        } else if (c === 27) {
            style.inverse = false;
        } else if (c >= 30 && c <= 37) {
            style.fg = c - 30;
        } else if (c >= 90 && c <= 97) {
            style.fg = c - 90 + 8;
        } else if (c === 39) {
            style.fg = null;
        } else if (c >= 40 && c <= 47) {
            style.bg = c - 40;
        } else if (c >= 100 && c <= 107) {
            style.bg = c - 100 + 8;
        } else if (c === 49) {
            style.bg = null;
        } else if (c === 38 || c === 48) {
            let value = null;
            if (codes[i + 1] === 5 && i + 2 < codes.length) {
                const n = codes[i + 2];
                value = n < 16 ? n : color256(n);
                i += 2;
            } else if (codes[i + 1] === 2 && i + 4 < codes.length) {
                value = [codes[i + 2], codes[i + 3], codes[i + 4]];
                i += 4;
            }
            if (c === 38) {
                style.fg = value;
            } else {
                style.bg = value;
            }
        }
    }
}

// AnsiParser turns lines of the colour log into their text and spans. The
// style carries on to the next line, as in a terminal, but never colours
// wakeci's time prefix.
export class AnsiParser {
    constructor() {
        this.style = plainStyle();
        this.link = null;
    }

    // line parses one line, without its line break, into { text, spans }:
    // spans are { from, to, style, href } within text, only where the
    // style isn't plain or a link is open
    line(line) {
        const spans = [];
        let text = "";
        let last = 0;
        let start = 0;
        const textStart = prefixEnd(line.replace(SEQUENCE, ""));
        const close = () => {
            const from = Math.max(start, textStart);
            if (text.length > from && (!isPlain(this.style) || this.link)) {
                spans.push({ from, to: text.length, style: { ...this.style }, href: this.link });
            }
            start = text.length;
        };
        SEQUENCE.lastIndex = 0;
        for (let m = SEQUENCE.exec(line); m !== null; m = SEQUENCE.exec(line)) {
            text += line.slice(last, m.index);
            last = SEQUENCE.lastIndex;
            close();
            if (m[1] !== undefined) {
                applySGR(this.style, m[1]);
            } else {
                // Only web links, as for links in plain text
                this.link = /^https?:\/\//i.test(m[2]) ? m[2] : null;
            }
        }
        text += line.slice(last);
        close();
        return { text, spans };
    }
}

import { test, expect } from "@playwright/test";
import { AnsiParser, color256, prefixEnd } from "../src/log/ansi.js";

// The log viewer's parser of the colour log (B4)
test("colours become spans over the visible text", () => {
    const { text, spans } = new AnsiParser().line("[    1.200s] \x1b[1;31mFAIL\x1b[0m checkout \x1b[38;5;214mwarn\x1b[39m");
    expect(text).toBe("[    1.200s] FAIL checkout warn");
    expect(spans.map((s) => [text.slice(s.from, s.to), s.style.fg, s.style.bold])).toEqual([
        ["FAIL", 1, true],
        ["warn", [255, 175, 0], false],
    ]);
});

test("a style carries on to the next line, but not onto its time prefix", () => {
    const parser = new AnsiParser();
    parser.line("[ 0.1s] \x1b[32mok one");
    const { text, spans } = parser.line("[ 0.2s] ok two\x1b[0m");
    expect(spans).toHaveLength(1);
    expect(text.slice(spans[0].from, spans[0].to)).toBe("ok two");
    expect(parser.line("[ 0.3s] plain").spans).toEqual([]);
});

test("24-bit colours, backgrounds and resets", () => {
    const { spans } = new AnsiParser().line("\x1b[38;2;10;20;30;48;5;1mx\x1b[49my\x1b[39mz");
    expect(spans.map((s) => [s.from, s.style.fg, s.style.bg])).toEqual([
        [0, [10, 20, 30], 1],
        [1, [10, 20, 30], null],
    ]);
});

test("web hyperlinks become link spans, others are dropped", () => {
    const parser = new AnsiParser();
    const { text, spans } = parser.line("see \x1b]8;;https://example.com/r\x1b\\report\x1b]8;;\x1b\\ and \x1b]8;;file:///etc\x1b\\file\x1b]8;;\x1b\\");
    expect(text).toBe("see report and file");
    expect(spans).toEqual([expect.objectContaining({ from: 4, to: 10, href: "https://example.com/r" })]);
});

test("the time prefix and the 256-colour table", () => {
    expect(prefixEnd("[    1.200s] text")).toBe(13);
    expect(prefixEnd("text")).toBe(0);
    expect(color256(15)).toBeNull();
    expect(color256(16)).toEqual([0, 0, 0]);
    expect(color256(231)).toEqual([255, 255, 255]);
    expect(color256(244)).toEqual([128, 128, 128]);
});

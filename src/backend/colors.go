package main

import (
	"regexp"
	"strings"
)

// keptEscapeRE matches what a task's colour log (task_N.ansi.log) keeps of a
// line: SGR codes (colour and style) and terminal hyperlinks (OSC 8)
var keptEscapeRE = regexp.MustCompile(`^(?:\x1b\[[0-9;:]*m|\x1b\]8;[^;\x07\x1b]*;[^\x07\x1b]*(?:\x07|\x1b\\))$`)

// anyEscapeRE matches every escape sequence a line may hold: the kept ones,
// other OSC sequences, and what StripColor removes
var anyEscapeRE = regexp.MustCompile(`\x1b\[[0-9;:]*m|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|` + colorEscapeCodes)

// colorCode is an escape sequence the colour log keeps, before the visible
// byte at pos
type colorCode struct {
	pos  int
	text string
}

// colorLine returns a line of task output twice, with secrets redacted:
// plain, its visible text, for task_N.log, and ansi, the same text with its
// colour codes and hyperlinks, for task_N.ansi.log. colored reports whether
// the line had any. Secrets are redacted in the visible text, so colour
// codes inside a secret can't hide it; the codes stay around the redaction.
// Every other escape sequence is dropped from both.
func colorLine(line string) (plain, ansi string, colored bool) {
	var visible strings.Builder
	var codes []colorCode
	last := 0
	for _, m := range anyEscapeRE.FindAllStringIndex(line, -1) {
		visible.WriteString(line[last:m[0]])
		if seq := line[m[0]:m[1]]; keptEscapeRE.MatchString(seq) {
			codes = append(codes, colorCode{visible.Len(), seq})
		}
		last = m[1]
	}
	visible.WriteString(line[last:])
	plain = redactSecrets(visible.String())
	if len(codes) == 0 {
		return plain, plain, false
	}
	return plain, redactAroundCodes(visible.String(), codes), true
}

// redactAroundCodes redacts the secrets in visible as redactSecrets does,
// value by value, and puts codes back: those inside a redacted secret go
// before the redaction
func redactAroundCodes(visible string, codes []colorCode) string {
	// span[i] is the range of visible's bytes that text[i] stands for
	type span struct{ from, to int }
	text := []byte(visible)
	spans := make([]span, len(text))
	for i := range spans {
		spans[i] = span{i, i + 1}
	}
	for _, value := range secretRedactionValues() {
		if value == "" || !strings.Contains(string(text), value) {
			continue
		}
		out := make([]byte, 0, len(text))
		outSpans := make([]span, 0, len(text))
		for i := 0; i < len(text); {
			if strings.HasPrefix(string(text[i:]), value) {
				covered := span{spans[i].from, spans[i+len(value)-1].to}
				for j := 0; j < len(redactedSecret); j++ {
					out = append(out, redactedSecret[j])
					outSpans = append(outSpans, covered)
				}
				i += len(value)
				continue
			}
			out = append(out, text[i])
			outSpans = append(outSpans, spans[i])
			i++
		}
		text, spans = out, outSpans
	}
	var b strings.Builder
	next := 0
	for i, c := range text {
		// Before the first byte a span gives, the codes before its end
		if i == 0 || spans[i] != spans[i-1] {
			for next < len(codes) && codes[next].pos < spans[i].to {
				b.WriteString(codes[next].text)
				next++
			}
		}
		b.WriteByte(c)
	}
	for ; next < len(codes); next++ {
		b.WriteString(codes[next].text)
	}
	return b.String()
}

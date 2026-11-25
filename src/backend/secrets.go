package main

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// secretsRegex matches exactly one `{{ secrets.NAME }}` reference at a time.
var secretsRegex = regexp.MustCompile(`{{\s*secrets\.([A-Za-z0-9_]+)\s*}}`)

const redactedSecret = "***REDACTED***"

// injectSecrets replaces every `{{ secrets.NAME }}` reference in str with the
// named secret's value (or the literal "<no value>" if NAME isn't
// configured, matching text/template's own missing-key rendering).
//
// This deliberately does its own targeted substitution instead of running
// str through text/template: executing it as a template against
// Config.secrets (the previous approach) let a single {{ secrets.X }}
// reference - required only to pass the "does this string use secrets"
// check - unlock arbitrary template actions elsewhere in the same string,
// including referencing a *different* secret than the one named, or a
// {{ range $k, $v := . }} that dumped every configured secret at once. A
// plain regex substitution can only ever resolve the one named key.
func injectSecrets(str string) string {
	return secretsRegex.ReplaceAllStringFunc(str, func(match string) string {
		key := secretsRegex.FindStringSubmatch(match)[1]
		if value, ok := Config.secrets[key]; ok {
			return value
		}
		return "<no value>"
	})
}

func injectCommandSecrets(command string) (string, []string) {
	matches := secretsRegex.FindAllStringSubmatchIndex(command, -1)
	if len(matches) == 0 {
		return command, nil
	}

	var transformed strings.Builder
	transformed.Grow(len(command))
	env := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	quote := byte(0)
	last := 0
	for _, match := range matches {
		segment := command[last:match[0]]
		transformed.WriteString(segment)
		quote = shellQuoteAfter(segment, quote)

		key := command[match[2]:match[3]]
		envName := "WAKE_SECRET_" + key
		switch quote {
		case '\'':
			transformed.WriteString("'\"${")
			transformed.WriteString(envName)
			transformed.WriteString("}\"'")
		case '"':
			transformed.WriteString("${" + envName + "}")
		default:
			transformed.WriteString("\"${" + envName + "}\"")
		}

		if _, ok := seen[key]; !ok {
			value := "<no value>"
			if configured, ok := Config.secrets[key]; ok {
				value = configured
			}
			env = append(env, envName+"="+value)
			seen[key] = struct{}{}
		}
		last = match[1]
	}
	transformed.WriteString(command[last:])
	return transformed.String(), env
}

func shellQuoteAfter(segment string, quote byte) byte {
	escaped := false
	for i := 0; i < len(segment); i++ {
		char := segment[i]
		if quote == '\'' {
			if char == '\'' {
				quote = 0
			}
			continue
		}
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if char == quote {
			quote = 0
			continue
		}
		if quote == 0 && (char == '\'' || char == '"') {
			quote = char
		}
	}
	return quote
}

func secretRedactionValues() []string {
	values := make([]string, 0, len(Config.secrets))
	seen := make(map[string]struct{}, len(Config.secrets))
	for _, secret := range Config.secrets {
		for _, value := range strings.FieldsFunc(secret, func(r rune) bool {
			return r == '\r' || r == '\n'
		}) {
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	values = append(values, buildTokenValues()...)
	sort.Slice(values, func(i, j int) bool {
		return len(values[i]) > len(values[j])
	})
	return values
}

// redactSecrets redacts configured secret values from build log text. Output
// arrives one line at a time, so multiline secrets are redacted by their
// non-empty line components. Longer values are replaced first to avoid
// leaking the suffix of a secret that contains another secret as a prefix.
func redactSecrets(str string) string {
	values := secretRedactionValues()
	for _, value := range values {
		str = strings.ReplaceAll(str, value, redactedSecret)
	}
	return str
}

// copyRedactedSecrets copies src to dst while replacing exact configured
// secret values. It retains enough bytes between reads to detect values that
// cross an I/O-buffer boundary, without loading the whole artifact in memory.
func copyRedactedSecrets(dst io.Writer, src io.Reader) error {
	stringValues := secretRedactionValues()
	if len(stringValues) == 0 {
		if _, err := io.Copy(dst, src); err != nil {
			return fmt.Errorf("copy unredacted data: %w", err)
		}
		return nil
	}

	values := make([][]byte, 0, len(stringValues))
	maxValueLen := 0
	for _, value := range stringValues {
		values = append(values, []byte(value))
		maxValueLen = max(maxValueLen, len(value))
	}

	readBuffer := make([]byte, 32*1024)
	pending := make([]byte, 0, len(readBuffer)+maxValueLen)
	for {
		n, readErr := src.Read(readBuffer)
		pending = append(pending, readBuffer[:n]...)

		consumed, err := writeRedactedSecrets(dst, pending, values, maxValueLen, false)
		if err != nil {
			return err
		}
		pending = append(pending[:0], pending[consumed:]...)

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read data to redact: %w", readErr)
		}
	}

	if _, err := writeRedactedSecrets(dst, pending, values, maxValueLen, true); err != nil {
		return err
	}
	return nil
}

func writeRedactedSecrets(
	dst io.Writer,
	data []byte,
	values [][]byte,
	maxValueLen int,
	final bool,
) (int, error) {
	limit := len(data)
	if !final {
		limit -= maxValueLen - 1
		if limit <= 0 {
			return 0, nil
		}
	}

	i := 0
	literalStart := 0
	for i < limit {
		var matchedValue []byte
		for _, value := range values {
			if !bytes.HasPrefix(data[i:], value) {
				continue
			}
			matchedValue = value
			break
		}
		if matchedValue == nil {
			i++
			continue
		}

		if err := writeAll(dst, data[literalStart:i]); err != nil {
			return i, fmt.Errorf("write artifact data: %w", err)
		}
		if err := writeAll(dst, []byte(redactedSecret)); err != nil {
			return i, fmt.Errorf("write redacted value: %w", err)
		}
		i += len(matchedValue)
		literalStart = i
	}
	if err := writeAll(dst, data[literalStart:i]); err != nil {
		return i, fmt.Errorf("write artifact data: %w", err)
	}
	return i, nil
}

func writeAll(dst io.Writer, data []byte) error {
	written, err := dst.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

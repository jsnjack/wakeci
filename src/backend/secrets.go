package main

import (
	"bytes"
	"regexp"
	"strings"
	"text/template"
)

var secretsRegex = regexp.MustCompile(`{{\s*secrets\.`)

const redactedSecret = "***REDACTED***"

// injectSecrets is a function that injects secrets into the string
func injectSecrets(str string) string {
	// text/template doesn't support nested keys, so we substitute secrets. part
	preparedStringBytes := secretsRegex.ReplaceAll([]byte(str), []byte("{{."))
	preparedString := string(preparedStringBytes)
	if preparedString == str {
		return str
	}
	tpl, err := template.New("injectSecrets").Parse(preparedString)
	if err != nil {
		return str
	}
	writer := new(bytes.Buffer)
	err = tpl.Execute(writer, Config.secrets)
	if err != nil {
		return str
	}
	return writer.String()
}

// redactSecrets is a function that redacts secrets from the string (build logs, etc.)
func redactSecrets(str string) string {
	for _, value := range Config.secrets {
		str = strings.ReplaceAll(str, value, redactedSecret)
	}
	return str
}

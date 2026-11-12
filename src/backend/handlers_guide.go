package main

import (
	_ "embed"
	"net/http"
)

// agentGuide tells agents and scripts how to use the API, at /llms.txt
//
//go:embed llms.txt
var agentGuide []byte

// HandleAgentGuide serves the guide for agents. It is public, and also
// served to requests with an API key.
func HandleAgentGuide(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	writeBody(requestLogger(r), w, agentGuide)
}

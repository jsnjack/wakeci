package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// HandleAPIDocsView renders swagger UI
func HandleAPIDocsView(w http.ResponseWriter, r *http.Request) {
	indexDoc := `
<!DOCTYPE html>
<html lang="en">
<head>
	<title>wakeci - API</title>
	<script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
	<link href="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui.css" rel="stylesheet">
</head>
<body>
	<div id="swagger-container"><div>
	<script>
		const ui = SwaggerUIBundle({
			url: "/docs/swagger.json",
			dom_id: "#swagger-container",
		})
	</script>
</body>
</html>
`
	w.Header().Set("content-security-policy", "default-src 'self'; script-src 'self' https://unpkg.com 'sha256-vRXqJZN28RidhIcwjq/UDO9cWHiPP1H4fdwi42L1TOM='; style-src 'self' https://unpkg.com; img-src 'self' data:; frame-ancestors 'self'")
	w.Header().Set("content-type", "text/html")
	writeBody(L, w, []byte(indexDoc))
}

// MCPToolDoc describes an MCP tool for the help page
type MCPToolDoc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Scope is the API key scope the tool needs, any for none
	Scope  string        `json:"scope"`
	Inputs []MCPInputDoc `json:"inputs"`
}

// MCPInputDoc is one input of an MCP tool
type MCPInputDoc struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

var (
	mcpDocsOnce sync.Once
	mcpDocsData []byte
	mcpDocsErr  error
)

// mcpToolDocs asks the MCP server for its tools, as a client would, so the
// help page describes the tools as they are
func mcpToolDocs(ctx context.Context) ([]MCPToolDoc, error) {
	server := newMCPServer(&mcpCaller{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("start mcp server: %w", err)
	}
	defer closeMCPSession(serverSession.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "wakeci-docs", Version: Version}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to mcp server: %w", err)
	}
	defer closeMCPSession(clientSession.Close)
	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list mcp tools: %w", err)
	}
	docs := []MCPToolDoc{}
	for _, tool := range result.Tools {
		scope, _ := tool.Meta["scope"].(string)
		doc := MCPToolDoc{Name: tool.Name, Description: tool.Description, Scope: scope, Inputs: []MCPInputDoc{}}
		var schema struct {
			Properties map[string]struct {
				Type        any    `json:"type"`
				Description string `json:"description"`
			} `json:"properties"`
			Required []string `json:"required"`
		}
		data, err := json.Marshal(tool.InputSchema)
		if err == nil {
			err = json.Unmarshal(data, &schema)
		}
		if err != nil {
			return nil, fmt.Errorf("read the inputs of %s: %w", tool.Name, err)
		}
		for name, property := range schema.Properties {
			doc.Inputs = append(doc.Inputs, MCPInputDoc{
				Name: name, Type: schemaType(property.Type), Description: property.Description,
				Required: slices.Contains(schema.Required, name),
			})
		}
		// Required inputs first, then by name, so the page is stable
		slices.SortFunc(doc.Inputs, func(a, b MCPInputDoc) int {
			if a.Required != b.Required {
				if a.Required {
					return -1
				}
				return 1
			}
			return strings.Compare(a.Name, b.Name)
		})
		docs = append(docs, doc)
	}
	slices.SortFunc(docs, func(a, b MCPToolDoc) int { return strings.Compare(a.Name, b.Name) })
	return docs, nil
}

// schemaType names a JSON schema type, which is a name or a list of names
func schemaType(t any) string {
	switch t := t.(type) {
	case string:
		return t
	case []any:
		var names []string
		for _, name := range t {
			if name, ok := name.(string); ok && name != "null" {
				names = append(names, name)
			}
		}
		return strings.Join(names, " or ")
	}
	return ""
}

func closeMCPSession(closeSession func() error) {
	if err := closeSession(); err != nil {
		L.Log(context.Background(), LevelTrace, "close mcp docs session", "err", err)
	}
}

// HandleMCPDocs returns the MCP tools with their inputs and scopes, for the
// help page. It is public, like the rest of the docs.
func HandleMCPDocs(w http.ResponseWriter, r *http.Request) {
	mcpDocsOnce.Do(func() {
		// Not the request's context: a client that goes away mustn't leave
		// an error behind for every later request
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var docs []MCPToolDoc
		docs, mcpDocsErr = mcpToolDocs(ctx)
		if mcpDocsErr == nil {
			mcpDocsData, mcpDocsErr = json.Marshal(map[string]any{"endpoint": "/mcp", "tools": docs})
		}
	})
	logger := requestLogger(r)
	if mcpDocsErr != nil {
		logger.Error("describe mcp tools", "err", mcpDocsErr)
		writePlainText(w, logger, http.StatusInternalServerError, mcpDocsErr.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeBody(logger, w, mcpDocsData)
}

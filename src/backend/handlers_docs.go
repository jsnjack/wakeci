package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// apiDocsScript draws the REST API docs with Scalar. In the Help page's
// frame they take the app's colours (BeerCSS tokens) and theme and follow
// its toggle; opened on their own they follow the system's theme. Scalar's
// AI agent, MCP generator, developer tools and telemetry talk to scalar.com,
// so they are off (and the CSP would block them).
const apiDocsScript = `
const app = (() => {
	try {
		return window.parent !== window && window.parent.document.body ? window.parent : null;
	} catch {
		return null;
	}
})();
// Scalar's colours and the app's tokens they come from
const tokens = {
	"--scalar-background-1": "--surface",
	"--scalar-background-2": "--surface-container",
	"--scalar-background-3": "--surface-container-highest",
	"--scalar-background-accent": "--primary-container",
	"--scalar-color-1": "--on-surface",
	"--scalar-color-2": "--on-surface-variant",
	"--scalar-color-3": "--outline",
	"--scalar-color-accent": "--primary",
	"--scalar-color-red": "--error",
	"--scalar-border-color": "--outline-variant",
	"--scalar-button-1": "--primary",
	"--scalar-button-1-color": "--on-primary",
	"--scalar-button-1-hover": "--primary",
	"--scalar-sidebar-background-1": "--surface-container-low",
	"--scalar-sidebar-color-1": "--on-surface",
	"--scalar-sidebar-color-2": "--on-surface-variant",
	"--scalar-sidebar-color-active": "--on-secondary-container",
	"--scalar-sidebar-item-hover-background": "--surface-container-high",
	"--scalar-sidebar-item-active-background": "--secondary-container",
	"--scalar-sidebar-border-color": "--outline-variant",
	"--scalar-sidebar-search-background": "--surface-container",
	"--scalar-sidebar-search-border-color": "--outline-variant",
	"--scalar-sidebar-search-color": "--on-surface-variant",
};
const placeholderCss = 'input[placeholder="QUxMIFlPVVIgQkFTRSBBUkUgQkVMT05HIFRPIFVT"]::placeholder { color: transparent; }';
function configuration() {
	const config = {
		url: "/docs/swagger.json",
		telemetry: false,
		withDefaultFonts: false,
		showDeveloperTools: "never",
		// Scalar's placeholder for a token is a joke in base64, which looks
		// like a real key
		customCss: placeholderCss,
		agent: { disabled: true },
		mcp: { disabled: true },
		documentDownloadType: "json",
		authentication: { preferredSecurityScheme: "APIKey" },
		defaultHttpClient: { targetKey: "shell", clientKey: "curl" },
	};
	if (!app) {
		return config;
	}
	const style = app.getComputedStyle(app.document.body);
	const colours = Object.entries(tokens)
		.map(([scalar, token]) => scalar + ": " + style.getPropertyValue(token).trim() + ";")
		.join(" ");
	config.hideDarkModeToggle = true;
	config.forceDarkModeState = app.document.body.classList.contains("dark") ? "dark" : "light";
	config.customCss += " .light-mode, .dark-mode { " + colours + " --scalar-font: " + style.fontFamily + "; }";
	return config;
}
const reference = Scalar.createApiReference("#app", configuration());
if (app) {
	// Scalar reads forceDarkModeState only when it starts, so a later
	// change of theme sets its body classes here
	new MutationObserver(() => {
		const config = configuration();
		reference.updateConfiguration(config);
		document.body.classList.toggle("dark-mode", config.forceDarkModeState === "dark");
		document.body.classList.toggle("light-mode", config.forceDarkModeState === "light");
	}).observe(app.document.body, { attributes: true, attributeFilter: ["class", "style"] });
}
`

// scalarScript is Scalar's standalone bundle; the hash is of the npm
// package's file, which jsDelivr serves unchanged
const (
	scalarScript          = "https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.76.2/dist/browser/standalone.js"
	scalarScriptIntegrity = "sha384-mCh2A/B6vkfhiHyzwBZm7lbFmEK5fE/96Kbtk+RH1zei1P/XEt0I2MCiG6WfZdor"
)

var (
	apiDocsPage = `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>wakeci - API</title>
	<meta name="description" content="The wakeci REST API. Without JavaScript, read it as Markdown at /llms-full.txt (also /docs/api.md), or as OpenAPI 2.0 at /docs/swagger.json.">
	<link rel="alternate" type="text/markdown" href="/llms-full.txt">
</head>
<body>
	<p hidden>This page draws the REST API docs with JavaScript. Agents and scripts can read them as Markdown at <a href="/llms-full.txt">/llms-full.txt</a> (also <a href="/docs/api.md">/docs/api.md</a>), or as OpenAPI 2.0 at <a href="/docs/swagger.json">/docs/swagger.json</a>.</p>
	<div id="app"></div>
	<script src="` + scalarScript + `" integrity="` + scalarScriptIntegrity + `" crossorigin="anonymous"></script>
	<script>` + apiDocsScript + `</script>
</body>
</html>
`
	// Scalar adds its styles as style elements, hence 'unsafe-inline' for
	// styles; scripts are only the pinned bundle and the script above, and
	// requests only go to wakeci
	apiDocsCSP = "default-src 'self'; script-src " + scalarScript + " '" + scriptHash(apiDocsScript) +
		"'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; " +
		"frame-ancestors 'self'; base-uri 'none'; form-action 'self'"
)

// scriptHash returns the CSP source that allows an inline script
func scriptHash(script string) string {
	sum := sha256.Sum256([]byte(script))
	return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
}

// HandleAPIDocsView shows the REST API docs, drawn from /docs/swagger.json
func HandleAPIDocsView(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", apiDocsCSP)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	writeBody(L, w, []byte(apiDocsPage))
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

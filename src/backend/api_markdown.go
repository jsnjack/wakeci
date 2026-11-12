package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
)

// The REST API reference as Markdown, for agents and scripts that can't run
// the docs page's JavaScript and shouldn't have to read the whole spec. It
// is built once from the embedded spec, so it lists what the spec lists.

// openAPISpec is the part of the Swagger 2.0 spec the reference shows
type openAPISpec struct {
	Info struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	} `json:"info"`
	Paths       map[string]map[string]*specOperation `json:"paths"`
	Definitions map[string]*specSchema               `json:"definitions"`
}

type specOperation struct {
	Summary     string                  `json:"summary"`
	Description string                  `json:"description"`
	Tags        []string                `json:"tags"`
	Produces    []string                `json:"produces"`
	Parameters  []specParameter         `json:"parameters"`
	Responses   map[string]specResponse `json:"responses"`
}

type specParameter struct {
	Name        string      `json:"name"`
	In          string      `json:"in"`
	Type        string      `json:"type"`
	Items       *specSchema `json:"items"`
	Required    bool        `json:"required"`
	Description string      `json:"description"`
}

type specResponse struct {
	Schema *specSchema `json:"schema"`
}

type specSchema struct {
	Ref                  string                 `json:"$ref"`
	Type                 string                 `json:"type"`
	Items                *specSchema            `json:"items"`
	AdditionalProperties *specSchema            `json:"additionalProperties"`
	Properties           map[string]*specSchema `json:"properties"`
	// AllOf holds the type of a field that has a description of its own
	AllOf       []*specSchema `json:"allOf"`
	Description string        `json:"description"`
	Nullable    bool          `json:"x-nullable"`
}

// commonStatuses are the answers every route can give, which the
// reference names once instead of on every route
var commonStatuses = []string{"401", "403", "429"}

// methodOrder sorts the methods of a path
var methodOrder = []string{"get", "head", "post", "put", "patch", "delete"}

var (
	apiMarkdownOnce sync.Once
	apiMarkdownData []byte
	apiMarkdownErr  error
)

// HandleAPIMarkdown serves the REST API reference as Markdown, at
// /docs/api.md and /llms-full.txt. Like the spec, it needs no credentials
func HandleAPIMarkdown(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	apiMarkdownOnce.Do(func() {
		var spec []byte
		if spec, apiMarkdownErr = APIDocs.ReadFile("docs/swagger.json"); apiMarkdownErr == nil {
			apiMarkdownData, apiMarkdownErr = apiMarkdown(spec)
		}
	})
	if apiMarkdownErr != nil {
		logger.Error("build the api reference", "err", apiMarkdownErr)
		writePlainText(w, logger, http.StatusInternalServerError, "Couldn't build the API reference")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	writeBody(logger, w, apiMarkdownData)
}

// apiMarkdown writes the reference of the spec
func apiMarkdown(specJSON []byte) ([]byte, error) {
	var spec openAPISpec
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return nil, fmt.Errorf("decode spec: %w", err)
	}
	inlineSimpleTypes(&spec)
	var out bytes.Buffer
	fmt.Fprintf(&out, "# %s\n\n", spec.Info.Title)
	for _, line := range strings.Split(spec.Info.Description, "\n") {
		fmt.Fprintf(&out, "%s\n", line)
	}
	fmt.Fprintf(&out, "\nPaths are relative to the server, e.g. `https://<host>/api/feed`. "+
		"Every route can also answer %s as said above; the routes don't repeat them. "+
		"Error answers are plain text. The types of the JSON answers are at the end. "+
		"The same as OpenAPI 2.0: `/docs/swagger.json`; a guide to common tasks: `/llms.txt`.\n",
		strings.Join(commonStatuses, ", "))

	// Operations by their first tag
	type operation struct {
		method, path string
		op           *specOperation
	}
	byTag := map[string][]operation{}
	for path, methods := range spec.Paths {
		for method, op := range methods {
			tag := "other"
			if len(op.Tags) > 0 {
				tag = op.Tags[0]
			}
			byTag[tag] = append(byTag[tag], operation{method, path, op})
		}
	}
	tags := make([]string, 0, len(byTag))
	for tag := range byTag {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	used := map[string]bool{}
	for _, tag := range tags {
		ops := byTag[tag]
		sort.Slice(ops, func(i, j int) bool {
			if ops[i].path != ops[j].path {
				return ops[i].path < ops[j].path
			}
			return slices.Index(methodOrder, ops[i].method) < slices.Index(methodOrder, ops[j].method)
		})
		fmt.Fprintf(&out, "\n## %s\n", tag)
		for _, o := range ops {
			writeOperation(&out, o.method, o.path, o.op, used)
		}
	}

	// The types the answers use, and the types those use
	for queue := sortedKeys(used); len(queue) > 0; {
		name := queue[0]
		queue = queue[1:]
		definition := spec.Definitions[name]
		if definition == nil {
			return nil, fmt.Errorf("type %s isn't defined", name)
		}
		for _, field := range definition.Properties {
			for _, ref := range schemaRefs(field) {
				if !used[ref] {
					used[ref] = true
					queue = append(queue, ref)
				}
			}
		}
	}
	if len(used) > 0 {
		out.WriteString("\n## Types\n")
	}
	for _, name := range sortedKeys(used) {
		fmt.Fprintf(&out, "\n### %s\n\n", typeLabel(name))
		definition := spec.Definitions[name]
		if len(definition.Properties) == 0 {
			fmt.Fprintf(&out, "%s\n", specTypeName(definition))
			continue
		}
		for _, field := range sortedKeys(definition.Properties) {
			schema := definition.Properties[field]
			fmt.Fprintf(&out, "- `%s` %s", field, specTypeName(schema))
			if description := oneLine(schema.Description); description != "" {
				fmt.Fprintf(&out, ": %s", description)
			}
			out.WriteString("\n")
		}
	}
	return out.Bytes(), nil
}

// writeOperation writes one route, and records in used the types its
// answers have
func writeOperation(out *bytes.Buffer, method, path string, op *specOperation, used map[string]bool) {
	fmt.Fprintf(out, "\n### %s %s\n\n", strings.ToUpper(method), path)
	if summary := sentence(op.Summary); summary != "" {
		fmt.Fprintf(out, "%s ", summary)
	}
	fmt.Fprintf(out, "%s\n", sentence(oneLine(op.Description)))
	if len(op.Parameters) > 0 {
		out.WriteString("\n")
	}
	for _, p := range op.Parameters {
		kind := p.Type
		if p.Type == "array" && p.Items != nil {
			kind = "[]" + p.Items.Type + ", repeat it for several"
		}
		in := p.In
		if in == "formData" {
			in = "form"
		}
		fmt.Fprintf(out, "- `%s` (%s, %s", p.Name, in, kind)
		if p.Required {
			out.WriteString(", required")
		}
		out.WriteString(")")
		if description := oneLine(p.Description); description != "" {
			fmt.Fprintf(out, ": %s", description)
		}
		out.WriteString("\n")
	}

	var answers, errors []string
	for _, status := range sortedKeys(op.Responses) {
		if slices.Contains(commonStatuses, status) {
			continue
		}
		if status >= "400" {
			errors = append(errors, status)
			continue
		}
		answer := status
		if schema := op.Responses[status].Schema; schema != nil && schema.Type != "string" {
			answer += " " + specTypeName(schema)
			for _, ref := range schemaRefs(schema) {
				used[ref] = true
			}
		} else if status != "304" && slices.Contains(op.Produces, "text/plain") {
			answer += " text"
		}
		answers = append(answers, answer)
	}
	fmt.Fprintf(out, "\nAnswers %s", strings.Join(answers, ", "))
	if len(errors) > 0 {
		fmt.Fprintf(out, "; errors %s", strings.Join(errors, ", "))
	}
	out.WriteString(".\n")
}

// inlineSimpleTypes replaces references to definitions without fields,
// such as time.Duration, by their type, so the reference doesn't list them
// as types
func inlineSimpleTypes(spec *openAPISpec) {
	var inline func(schema *specSchema)
	inline = func(schema *specSchema) {
		if schema == nil {
			return
		}
		// swag writes a described field's type as allOf with one schema
		if len(schema.AllOf) == 1 && schema.Ref == "" && schema.Type == "" {
			one := schema.AllOf[0]
			schema.Ref, schema.Type, schema.Items, schema.AllOf = one.Ref, one.Type, one.Items, nil
		}
		name := strings.TrimPrefix(schema.Ref, "#/definitions/")
		if definition := spec.Definitions[name]; schema.Ref != "" && definition != nil && len(definition.Properties) == 0 && definition.Type != "object" {
			schema.Ref, schema.Type = "", definition.Type
			if name == "time.Duration" {
				schema.Type += ", nanoseconds"
			}
		}
		inline(schema.Items)
		inline(schema.AdditionalProperties)
		for _, field := range schema.Properties {
			inline(field)
		}
	}
	for _, definition := range spec.Definitions {
		inline(definition)
	}
	for _, methods := range spec.Paths {
		for _, op := range methods {
			for _, response := range op.Responses {
				inline(response.Schema)
			}
		}
	}
}

// specTypeName names a schema's type, such as `[]BuildUpdateData`
func specTypeName(schema *specSchema) string {
	var name string
	switch {
	case schema.Ref != "":
		name = typeLabel(strings.TrimPrefix(schema.Ref, "#/definitions/"))
	case schema.Type == "array" && schema.Items != nil:
		name = "[]" + strings.Trim(specTypeName(schema.Items), "`")
	case schema.AdditionalProperties != nil:
		name = "map[string]" + strings.Trim(specTypeName(schema.AdditionalProperties), "`")
	case schema.Type != "":
		name = schema.Type
	default:
		name = "any"
	}
	name = "`" + name + "`"
	if schema.Nullable {
		name += " or null"
	}
	return name
}

// schemaRefs returns the definitions a schema refers to
func schemaRefs(schema *specSchema) []string {
	switch {
	case schema == nil:
		return nil
	case schema.Ref != "":
		return []string{strings.TrimPrefix(schema.Ref, "#/definitions/")}
	case schema.Items != nil:
		return schemaRefs(schema.Items)
	case schema.AdditionalProperties != nil:
		return schemaRefs(schema.AdditionalProperties)
	}
	return nil
}

// typeLabel drops the Go package from a definition's name
func typeLabel(name string) string {
	return strings.TrimPrefix(name, "main.")
}

// oneLine joins a description's lines
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// sentence ends text with a full stop
func sentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || strings.HasSuffix(text, ".") {
		return text
	}
	return text + "."
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

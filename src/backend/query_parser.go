package main

import (
	"regexp"
	"strconv"
	"strings"
)

type FilterRequest struct {
	MustInclude []string
	ContainsAny []string
	MustExclude []string
}

// CreateFilterRequest parses query and returns FilterRequest object
func CreateFilterRequest(query string) *FilterRequest {
	if strings.Trim(query, " \n\t") == "" {
		return nil
	}
	data := SplitFilterQuery(query)
	result := FilterRequest{}
	for _, el := range data {
		if strings.HasPrefix(el, "-") {
			result.MustExclude = append(result.MustExclude, unquote(strings.TrimPrefix(el, "-")))
		} else if strings.HasPrefix(el, "+") {
			result.MustInclude = append(result.MustInclude, unquote(strings.TrimPrefix(el, "+")))
		} else {
			result.ContainsAny = append(result.ContainsAny, unquote(el))
		}
	}
	return &result
}

func SplitFilterQuery(query string) []string {
	var result []string
	var current strings.Builder
	inQuote := false
	var quoteChar rune

	for _, r := range query {
		if r == '"' || r == '\'' {
			if !inQuote {
				inQuote = true
				quoteChar = r
			} else if r == quoteChar {
				inQuote = false
			}
			current.WriteRune(r)
		} else if r == ' ' && !inQuote {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}

func unquote(query string) string {
	if strings.HasPrefix(query, `"`) && strings.HasSuffix(query, `"`) {
		return query[1 : len(query)-1]
	}
	if strings.HasPrefix(query, `'`) && strings.HasSuffix(query, `'`) {
		return query[1 : len(query)-1]
	}
	return query
}

// matchesFilter tells whether build b matches filter. A term matches the
// text of the build (ToFilterMatchString) anywhere, unless it is NAME=value
// (exactTerm). The browser's matchesFilter (store/utils.js) works the same,
// checked by testdata/search_cases.json.
func matchesFilter(b *BuildUpdateData, filter *FilterRequest) bool {
	if filter == nil {
		return true
	}
	text := b.ToFilterMatchString()

	for _, item := range filter.MustInclude {
		if !termMatches(b, text, item) {
			return false
		}
	}
	for _, item := range filter.MustExclude {
		if termMatches(b, text, item) {
			return false
		}
	}

	// If the are no ContainsAny filters, match string
	contains := len(filter.ContainsAny) == 0
	for _, item := range filter.ContainsAny {
		if termMatches(b, text, item) {
			contains = true
			break
		}
	}
	return contains
}

// exactTerm is NAME=value, which matches a build whose id, name or status
// is value, or with a param equal to value named NAME, ignoring case, or
// NAME_LABEL: a label by its shown name, so branch=main finds BRANCH_LABEL.
// Else it is found as text, as other terms: a value can hold = too
// (ARGS=mode=fast)
var exactTerm = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)

func termMatches(b *BuildUpdateData, text, term string) bool {
	m := exactTerm.FindStringSubmatch(term)
	if m == nil {
		return strings.Contains(text, term)
	}
	name, value := m[1], m[2]
	switch strings.ToLower(name) {
	case "id":
		if strconv.Itoa(b.ID) == value {
			return true
		}
	case "name":
		if b.Name == value {
			return true
		}
	case "status":
		if string(b.Status) == value {
			return true
		}
	}
	for _, param := range b.Params {
		for key, v := range param {
			if v == value && (strings.EqualFold(key, name) || strings.EqualFold(key, name+labelSuffix)) {
				return true
			}
		}
	}
	return strings.Contains(text, term)
}

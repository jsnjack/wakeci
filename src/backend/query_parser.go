package main

import (
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

func matchesFilter(s string, filter *FilterRequest) bool {
	if filter == nil {
		return true
	}

	for _, item := range filter.MustInclude {
		if !strings.Contains(s, item) {
			return false
		}
	}
	for _, item := range filter.MustExclude {
		if strings.Contains(s, item) {
			return false
		}
	}

	// If the are no ContainsAny filters, match string
	contains := len(filter.ContainsAny) == 0
	for _, item := range filter.ContainsAny {
		if strings.Contains(s, item) {
			contains = true
			break
		}
	}
	return contains
}

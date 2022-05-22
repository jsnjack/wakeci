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
	data := strings.Split(query, " ")
	for new := handleOpenQuotes(data, `"`); len(new) != len(data); {
		data = new
	}
	for new := handleOpenQuotes(data, `'`); len(new) != len(data); {
		data = new
	}
	return data
}

func handleOpenQuotes(data []string, quote string) []string {
	for i, el := range data {
		if strings.HasSuffix(el, quote) {
			if i > 0 {
				var new []string
				new = append(data[:i-1], strings.Join(data[i-1:i+1], " "))
				if len(data) > i+1 {
					new = append(new, data[i+1:]...)
				}
				return new
			}
		}
	}
	return data
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

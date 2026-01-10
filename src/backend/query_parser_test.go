package main

import (
	"testing"
)

func TestSplitFilterQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "DoubleQuotes",
			input:    `hello bye "good day"`,
			expected: []string{"hello", "bye", `"good day"`},
		},
		{
			name:     "SingleQuotes",
			input:    `hello bye 'good day'`,
			expected: []string{"hello", "bye", `'good day'`},
		},
		{
			name:     "OneQuote",
			input:    `hello bye good day"`,
			expected: []string{"hello", "bye", "good", `day"`},
		},
		{
			name:     "ThreeQuotes",
			input:    `hello 'bye "good day"`,
			expected: []string{"hello", `'bye "good day"`},
		},
		{
			name:     "FourQuotesInTheMiddle",
			input:    `hello b"y"e "good day"`,
			expected: []string{"hello", `b"y"e`, `"good day"`},
		},
		{
			name:     "OneWord",
			input:    `hello`,
			expected: []string{"hello"},
		},
		{
			name:     "OneWordWithSign",
			input:    `-hello`,
			expected: []string{"-hello"},
		},
		{
			name:     "OneWordWithSignAndQuotes1",
			input:    `-"hello"`,
			expected: []string{`-"hello"`},
		},
		{
			name:     "OneWordWithSignAndQuotes2",
			input:    `"-hello"`,
			expected: []string{`"-hello"`},
		},
		{
			name:     "FourQuotes2",
			input:    `""hello""`,
			expected: []string{`""hello""`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitFilterQuery(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d elements, got %d (%v)", len(tt.expected), len(result), result)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("At index %d: expected %s, got %s", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

func TestCreateFilterRequest(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedAny  int
		expectedMust int
		expectedExcl int
	}{
		{
			name:         "OneWord",
			input:        "hello",
			expectedAny:  1,
			expectedMust: 0,
			expectedExcl: 0,
		},
		{
			name:         "TwoWords",
			input:        "hello bye",
			expectedAny:  2,
			expectedMust: 0,
			expectedExcl: 0,
		},
		{
			name:         "Include",
			input:        "+bye",
			expectedAny:  0,
			expectedMust: 1,
			expectedExcl: 0,
		},
		{
			name:         "Include2",
			input:        "hello +bye",
			expectedAny:  1,
			expectedMust: 1,
			expectedExcl: 0,
		},
		{
			name:         "Include3",
			input:        `"hello joe" +bye`,
			expectedAny:  1,
			expectedMust: 1,
			expectedExcl: 0,
		},
		{
			name:         "Include4",
			input:        `"hello joe" +bye -'alabama banana'`,
			expectedAny:  1,
			expectedMust: 1,
			expectedExcl: 1,
		},
		{
			name:         "Include with must",
			input:        `+"days:mon tue wed"`,
			expectedAny:  0,
			expectedMust: 1,
			expectedExcl: 0,
		},
		{
			name:        "Empty1",
			input:       "",
			expectedAny: -1, // nil
		},
		{
			name:        "Empty2",
			input:       " ",
			expectedAny: -1, // nil
		},
		{
			name:        "Empty3",
			input:       " \n\t",
			expectedAny: -1, // nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateFilterRequest(tt.input)
			if tt.expectedAny == -1 {
				if result != nil {
					t.Errorf("Expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Error("Expected non-nil result")
				return
			}
			if len(result.ContainsAny) != tt.expectedAny {
				t.Errorf("Expected %d ContainsAny, got %d", tt.expectedAny, len(result.ContainsAny))
			}
			if len(result.MustInclude) != tt.expectedMust {
				t.Errorf("Expected %d MustInclude, got %d", tt.expectedMust, len(result.MustInclude))
			}
			if len(result.MustExclude) != tt.expectedExcl {
				t.Errorf("Expected %d MustExclude, got %d", tt.expectedExcl, len(result.MustExclude))
			}
		})
	}
}

func Test_matchesFilter(t *testing.T) {
	tests := []struct {
		name  string
		query string
		data  []struct {
			update   BuildUpdateData
			expected bool
		}
	}{
		{
			name:  "Required keyword",
			query: "+failed",
			data: []struct {
				update   BuildUpdateData
				expected bool
			}{
				{update: BuildUpdateData{Status: "failed"}, expected: true},
				{update: BuildUpdateData{Status: "success"}, expected: false},
			},
		},
		{
			name:  "OR logic with phrases",
			query: `failed "timed out"`,
			data: []struct {
				update   BuildUpdateData
				expected bool
			}{
				{update: BuildUpdateData{Status: "failed"}, expected: true},
				{update: BuildUpdateData{Status: "timed out"}, expected: true},
				{update: BuildUpdateData{Status: "success"}, expected: false},
			},
		},
		{
			name:  "Requirement and exclusion",
			query: `+failed -test_`,
			data: []struct {
				update   BuildUpdateData
				expected bool
			}{
				{update: BuildUpdateData{Status: "failed", Name: "regular build"}, expected: true},
				{update: BuildUpdateData{Status: "failed", Name: "test_build"}, expected: false},
			},
		},
		{
			name:  "Targeted attribute matching",
			query: `status:failed name:myjob`,
			data: []struct {
				update   BuildUpdateData
				expected bool
			}{
				{update: BuildUpdateData{Status: "failed"}, expected: true},
				{update: BuildUpdateData{Name: "myjob"}, expected: true},
				{update: BuildUpdateData{Status: "success", Name: "other"}, expected: false},
				{update: BuildUpdateData{Status: "success", Name: "failed"}, expected: false},
			},
		},
		{
			name:  "Complex multi-condition",
			query: `+status:failed -"test build" env:prod`,
			data: []struct {
				update   BuildUpdateData
				expected bool
			}{
				{
					update:   BuildUpdateData{Name: "regular build", Status: "failed", Params: []map[string]string{{"env": "prod"}}},
					expected: true,
				},
				{
					update:   BuildUpdateData{Name: "test build", Status: "failed", Params: []map[string]string{{"env": "prod"}}},
					expected: false,
				},
				{
					update:   BuildUpdateData{Name: "regular build", Status: "running", Params: []map[string]string{{"env": "prod"}}},
					expected: false,
				},
				{
					update:   BuildUpdateData{Name: "regular build", Status: "failed", Params: []map[string]string{{"env": "dev"}}},
					expected: false,
				},
				{
					update:   BuildUpdateData{Name: "regular failed build", Status: "success", Params: []map[string]string{{"env": "dev"}}},
					expected: false,
				},
			},
		},
		{
			name:  "Space separated param value",
			query: `+"days:mon tue wed"`,
			data: []struct {
				update   BuildUpdateData
				expected bool
			}{
				{update: BuildUpdateData{Status: "failed", Params: []map[string]string{{"days": "mon tue wed"}}}, expected: true},
				{update: BuildUpdateData{Status: "finished", Params: []map[string]string{{"days": "mon"}}}, expected: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := CreateFilterRequest(tt.query)
			for i, d := range tt.data {
				result := matchesFilter(d.update.ToFilterMatchString(), filter)
				if result != d.expected {
					t.Errorf("Subtest %d (%v): For query %q, expected %v, got %v", i, d.update, tt.query, d.expected, result)
				}
			}
		})
	}
}

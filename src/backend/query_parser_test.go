package main

import (
	"testing"
)

func TestSplitFilterQuery_DoubleQuotes(t *testing.T) {
	input := `hello bye "good day"`
	result := SplitFilterQuery(input)
	if len(result) != 3 {
		t.Errorf("Expected %d elements, got %s", len(result), result)
		return
	}
	if result[2] != `"good day"` {
		t.Errorf("Unexpected element: %s", result[2])
		return
	}
}

func TestSplitFilterQuery_SingleQuotes(t *testing.T) {
	input := `hello bye 'good day'`
	result := SplitFilterQuery(input)
	if len(result) != 3 {
		t.Errorf("Expected %d elements, got %s", len(result), result)
		return
	}
	if result[2] != `'good day'` {
		t.Errorf("Unexpected element: %s", result[2])
		return
	}
}

func TestSplitFilterQuery_OneQuote(t *testing.T) {
	input := `hello bye good day"`
	result := SplitFilterQuery(input)
	if len(result) != 3 {
		t.Errorf("Expected %d elements, got %s", len(result), result)
		return
	}
	if result[2] != `good day"` {
		t.Errorf("Unexpected element: %s", result[2])
		return
	}
}

func TestSplitFilterQuery_ThreeQuotes(t *testing.T) {
	input := `hello 'bye "good day"`
	result := SplitFilterQuery(input)
	if len(result) != 3 {
		t.Errorf("Expected %d elements, got %s", len(result), result)
		return
	}
	if result[1] != `'bye` {
		t.Errorf("Unexpected element: %s", result[1])
		return
	}
	if result[2] != `"good day"` {
		t.Errorf("Unexpected element: %s", result[2])
		return
	}
}

func TestSplitFilterQuery_FourQuotesInTheMiddle(t *testing.T) {
	input := `hello b"y"e "good day"`
	result := SplitFilterQuery(input)
	if len(result) != 3 {
		t.Errorf("Expected %d elements, got %s", len(result), result)
		return
	}
	if result[1] != `b"y"e` {
		t.Errorf("Unexpected element: %s", result[1])
		return
	}
	if result[2] != `"good day"` {
		t.Errorf("Unexpected element: %s", result[2])
		return
	}
}

func TestSplitFilterQuery_OneWord(t *testing.T) {
	input := `hello`
	result := SplitFilterQuery(input)
	if len(result) != 1 {
		t.Errorf("Expected %d elements, got %s", len(result), result)
		return
	}
}

func TestSplitFilterQuery_OneWordWithSign(t *testing.T) {
	input := `-hello`
	result := SplitFilterQuery(input)
	if len(result) != 1 {
		t.Errorf("Expected %d elements, got %s", len(result), result)
		return
	}
}

func TestSplitFilterQuery_OneWordWithSignAndQuotes1(t *testing.T) {
	input := `-"hello"`
	result := SplitFilterQuery(input)
	if len(result) != 1 {
		t.Errorf("Expected %d elements, got %s", len(result), result)
		return
	}
	if result[0] != input {
		t.Errorf("Unexpected element: %s", result[0])
		return
	}
}

func TestSplitFilterQuery_OneWordWithSignAndQuotes2(t *testing.T) {
	input := `"-hello"`
	result := SplitFilterQuery(input)
	if len(result) != 1 {
		t.Errorf("Expected %d elements, got %s", len(result), result)
		return
	}
	if result[0] != input {
		t.Errorf("Unexpected element: %s", result[0])
		return
	}
}

func TestSplitFilterQuery_FourQuotes2(t *testing.T) {
	input := `""hello""`
	result := SplitFilterQuery(input)
	if len(result) != 1 {
		t.Errorf("Expected %d elements, got %s", len(result), result)
		return
	}
	if result[0] != input {
		t.Errorf("Unexpected element: %s", result[0])
		return
	}
}

func TestCreateFilterRequest_OneWord(t *testing.T) {
	input := `hello`
	result := CreateFilterRequest(input)
	if len(result.ContainsAny) != 1 {
		t.Errorf("Expected 1, got %d", len(result.ContainsAny))
		return
	}
	if len(result.MustInclude) != 0 {
		t.Errorf("Expected 0, got %d", len(result.MustInclude))
		return
	}
	if len(result.MustExclude) != 0 {
		t.Errorf("Expected 0, got %d", len(result.MustExclude))
		return
	}
}

func TestCreateFilterRequest_TwoWords(t *testing.T) {
	input := `hello bye`
	result := CreateFilterRequest(input)
	if len(result.ContainsAny) != 2 {
		t.Errorf("Expected 2, got %d", len(result.ContainsAny))
		return
	}
	if len(result.MustInclude) != 0 {
		t.Errorf("Expected 0, got %d", len(result.MustInclude))
		return
	}
	if len(result.MustExclude) != 0 {
		t.Errorf("Expected 0, got %d", len(result.MustExclude))
		return
	}
}

func TestCreateFilterRequest_Include(t *testing.T) {
	input := `+bye`
	result := CreateFilterRequest(input)
	if len(result.ContainsAny) != 0 {
		t.Errorf("Expected 0, got %d", len(result.ContainsAny))
		return
	}
	if len(result.MustInclude) != 1 {
		t.Errorf("Expected 1, got %d", len(result.MustInclude))
		return
	}
	if len(result.MustExclude) != 0 {
		t.Errorf("Expected 0, got %d", len(result.MustExclude))
		return
	}
	if result.MustInclude[0] != "bye" {
		t.Errorf("Expected bye, got %s", result.MustInclude[0])
		return
	}
}

func TestCreateFilterRequest_Include2(t *testing.T) {
	input := `hello +bye`
	result := CreateFilterRequest(input)
	if len(result.ContainsAny) != 1 {
		t.Errorf("Expected 1, got %d", len(result.ContainsAny))
		return
	}
	if len(result.MustInclude) != 1 {
		t.Errorf("Expected 0, got %d", len(result.MustInclude))
		return
	}
	if len(result.MustExclude) != 0 {
		t.Errorf("Expected 0, got %d", len(result.MustExclude))
		return
	}
	if result.MustInclude[0] != "bye" {
		t.Errorf("Expected bye, got %s", result.MustInclude[0])
		return
	}
	if result.ContainsAny[0] != "hello" {
		t.Errorf("Expected hello, got %s", result.ContainsAny[0])
		return
	}
}

func TestCreateFilterRequest_Include3(t *testing.T) {
	input := `"hello joe" +bye`
	result := CreateFilterRequest(input)
	if len(result.ContainsAny) != 1 {
		t.Errorf("Expected 1, got %d", len(result.ContainsAny))
		return
	}
	if len(result.MustInclude) != 1 {
		t.Errorf("Expected 0, got %d", len(result.MustInclude))
		return
	}
	if len(result.MustExclude) != 0 {
		t.Errorf("Expected 0, got %d", len(result.MustExclude))
		return
	}
	if result.MustInclude[0] != "bye" {
		t.Errorf("Expected bye, got %s", result.MustInclude[0])
		return
	}
	if result.ContainsAny[0] != "hello joe" {
		t.Errorf("Expected hello joe, got %s", result.ContainsAny[0])
		return
	}
}

func TestCreateFilterRequest_Include4(t *testing.T) {
	input := `"hello joe" +bye -'alabama banana'`
	result := CreateFilterRequest(input)
	if len(result.ContainsAny) != 1 {
		t.Errorf("Expected 1, got %d", len(result.ContainsAny))
		return
	}
	if len(result.MustInclude) != 1 {
		t.Errorf("Expected 0, got %d", len(result.MustInclude))
		return
	}
	if len(result.MustExclude) != 1 {
		t.Errorf("Expected 1, got %d", len(result.MustExclude))
		return
	}
	if result.MustInclude[0] != "bye" {
		t.Errorf("Expected bye, got %s", result.MustInclude[0])
		return
	}
	if result.ContainsAny[0] != "hello joe" {
		t.Errorf("Expected hello joe, got %s", result.ContainsAny[0])
		return
	}
	if result.MustExclude[0] != "alabama banana" {
		t.Errorf("Expected alabama banana, got %s", result.ContainsAny[0])
		return
	}
}

func TestCreateFilterRequest_Empty1(t *testing.T) {
	input := ""
	result := CreateFilterRequest(input)
	if result != nil {
		t.Errorf("Expected nil, got %s", result)
		return
	}
}

func TestCreateFilterRequest_Empty2(t *testing.T) {
	input := " "
	result := CreateFilterRequest(input)
	if result != nil {
		t.Errorf("Expected nil, got %s", result)
		return
	}
}

func TestCreateFilterRequest_Empty3(t *testing.T) {
	input := " \n\t"
	result := CreateFilterRequest(input)
	if result != nil {
		t.Errorf("Expected nil, got %s", result)
		return
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

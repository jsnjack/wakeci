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

func TestMatchesFilter_1(t *testing.T) {
	input := "123 hello zorro pruzhany"
	result := matchesFilter(input, nil)
	if !result {
		t.Errorf("Expected to match")
	}
}

func TestMatchesFilter_2(t *testing.T) {
	input := "123 hello zorro pruzhany"
	filter := FilterRequest{ContainsAny: []string{"hello"}}
	result := matchesFilter(input, &filter)
	if !result {
		t.Errorf("Expected to match")
	}
}

func TestMatchesFilter_3(t *testing.T) {
	input := "123 hello zorro pruzhany"
	filter := FilterRequest{ContainsAny: []string{"zorro", "hello", "bingo"}}
	result := matchesFilter(input, &filter)
	if !result {
		t.Errorf("Expected to match")
	}
}

func TestMatchesFilter_4(t *testing.T) {
	input := "123 hello zorro pruzhany"
	filter := FilterRequest{MustInclude: []string{"hello"}}
	result := matchesFilter(input, &filter)
	if !result {
		t.Errorf("Expected to match")
	}
}

func TestMatchesFilter_5(t *testing.T) {
	input := "123 hello zorro pruzhany"
	filter := FilterRequest{MustInclude: []string{"zombie"}}
	result := matchesFilter(input, &filter)
	if result {
		t.Errorf("Expected to not match")
	}
}

func TestMatchesFilter_6(t *testing.T) {
	input := "123 hello zorro pruzhany"
	filter := FilterRequest{MustInclude: []string{"123"}, MustExclude: []string{"zorro"}}
	result := matchesFilter(input, &filter)
	if result {
		t.Errorf("Expected to not match")
	}
}

func TestMatchesFilter_7(t *testing.T) {
	input := "1293 @myjob1652038289063 finished"
	filter := FilterRequest{ContainsAny: []string{"joe"}}
	result := matchesFilter(input, &filter)
	if result {
		t.Errorf("Expected to not match")
	}
}

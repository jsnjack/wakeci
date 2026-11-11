package main

import "testing"

func TestGrantHasScope(t *testing.T) {
	selected := &Grant{Scopes: []string{ScopeRead, ScopeWrite}, Jobs: []string{"deploy"}}
	if !selected.HasScope(ScopeRead) || selected.HasScope(ScopeRun) {
		t.Error("read/run scopes wrong")
	}
	if selected.HasScope(ScopeWrite) {
		t.Error("write counted on a grant that doesn't cover all jobs")
	}
	all := &Grant{Scopes: []string{ScopeWrite}, AllJobs: true}
	if !all.HasScope(ScopeWrite) {
		t.Error("write not counted on a grant that covers all jobs")
	}
}

func TestMatchJobPattern(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"deploy", "deploy", true},
		{"deploy", "deploy2", false},
		{"*", "anything", true},
		{"test_*", "test_api", true},
		{"test_*", "test_", true},
		{"test_*", "api_test_x", false},
		{"*_prod", "web_prod", true},
		{"*_prod", "web_prod_old", false},
		{"a*b*b", "abb", true},
		{"a*b*b", "ab", false},
		{"a*b*c", "axxbyyc", true},
		{"a*b*c", "axxcyyb", false},
		{"a*b*c", "axxc", false},
		{"v?c", "vxc", false},
		{"v?c", "v?c", true},
		{"[ab]", "a", false},
		{"[ab]", "[ab]", true},
	}
	for _, tt := range tests {
		if got := matchJobPattern(tt.pattern, tt.name); got != tt.want {
			t.Errorf("matchJobPattern(%q, %q) = %t, want %t", tt.pattern, tt.name, got, tt.want)
		}
	}
}

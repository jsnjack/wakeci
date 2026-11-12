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

func TestAccessAllows(t *testing.T) {
	deployRead := Grant{Scopes: []string{ScopeRead}, Patterns: []string{"deploy*"}}
	prodRun := Grant{Scopes: []string{ScopeRun}, Jobs: []string{"deploy-prod"}}
	allRead := Grant{Scopes: []string{ScopeRead, ScopeRun}, AllJobs: true}
	tests := []struct {
		name   string
		access *Access
		scope  string
		job    string
		want   bool
	}{
		{"read from the pattern grant", &Access{Grants: []Grant{deployRead, prodRun}}, ScopeRead, "deploy-staging", true},
		{"no run from the read grant", &Access{Grants: []Grant{deployRead, prodRun}}, ScopeRun, "deploy-staging", false},
		{"run from the job grant", &Access{Grants: []Grant{deployRead, prodRun}}, ScopeRun, "deploy-prod", true},
		{"job no grant covers", &Access{Grants: []Grant{deployRead, prodRun}}, ScopeRead, "nightly", false},
		{"no grants", &Access{}, ScopeRead, "deploy", false},
		{"full access", fullAccess, ScopeWrite, "anything", true},
		{"within allows it", &Access{Grants: []Grant{allRead}, Within: &Access{Grants: []Grant{deployRead}}}, ScopeRead, "deploy", true},
		{"within lacks the scope", &Access{Grants: []Grant{allRead}, Within: &Access{Grants: []Grant{deployRead}}}, ScopeRun, "deploy", false},
		{"within lacks the job", &Access{Grants: []Grant{allRead}, Within: &Access{Grants: []Grant{deployRead}}}, ScopeRead, "nightly", false},
		{"within empty", &Access{Grants: []Grant{allRead}, Within: &Access{}}, ScopeRead, "deploy", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.access.Allows(tt.scope, tt.job); got != tt.want {
				t.Errorf("Allows(%s, %s) = %t, want %t", tt.scope, tt.job, got, tt.want)
			}
		})
	}
}

func TestAccessHasScopeAndEmpty(t *testing.T) {
	read := Grant{Scopes: []string{ScopeRead}, Jobs: []string{"deploy"}}
	tests := []struct {
		name      string
		access    *Access
		wantScope map[string]bool
		wantEmpty bool
	}{
		{"one grant", &Access{Grants: []Grant{read}}, map[string]bool{ScopeRead: true, ScopeRun: false, ScopeWrite: false}, false},
		{"no grants", &Access{}, map[string]bool{ScopeRead: false}, true},
		// Write without all jobs never counts, so this grant gives nothing
		{"write on some jobs", &Access{Grants: []Grant{{Scopes: []string{ScopeWrite}, Jobs: []string{"deploy"}}}}, map[string]bool{ScopeWrite: false}, true},
		{"within takes scopes away", &Access{Grants: []Grant{read}, Within: &Access{}}, map[string]bool{ScopeRead: false}, true},
		{"full access", fullAccess, map[string]bool{ScopeRead: true, ScopeWrite: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for scope, want := range tt.wantScope {
				if got := tt.access.HasScope(scope); got != want {
					t.Errorf("HasScope(%s) = %t, want %t", scope, got, want)
				}
			}
			if got := tt.access.Empty(); got != tt.wantEmpty {
				t.Errorf("Empty() = %t, want %t", got, tt.wantEmpty)
			}
		})
	}
}

func TestAccessCheckWithin(t *testing.T) {
	groups := &Access{Grants: []Grant{
		{Scopes: []string{ScopeRead, ScopeRun}, Jobs: []string{"deploy"}},
		{Scopes: []string{ScopeRead}, Patterns: []string{"test_*"}},
	}}
	tests := []struct {
		name    string
		grant   Grant
		wantErr string
	}{
		{"same scopes on a covered job", Grant{Scopes: []string{ScopeRead, ScopeRun}, Jobs: []string{"deploy"}}, ""},
		{"read on a job a pattern covers", Grant{Scopes: []string{ScopeRead}, Jobs: []string{"test_api"}}, ""},
		// Patterns and all jobs are limited on every request instead
		{"a pattern", Grant{Scopes: []string{ScopeRead}, Patterns: []string{"*"}}, ""},
		{"all jobs", Grant{Scopes: []string{ScopeRead}, AllJobs: true}, ""},
		{"a scope the groups lack", Grant{Scopes: []string{ScopeWrite}, AllJobs: true}, "your groups don't have the write scope"},
		{"run on a job only read covers", Grant{Scopes: []string{ScopeRun}, Jobs: []string{"test_api"}}, "your groups don't allow run on test_api"},
		{"a job no group covers", Grant{Scopes: []string{ScopeRead}, Jobs: []string{"nightly"}}, "your groups don't allow read on nightly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := groups.checkWithin(&tt.grant)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("checkWithin() = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("checkWithin() = %v, want %q", err, tt.wantErr)
			}
		})
	}
	if err := fullAccess.checkWithin(&Grant{Scopes: []string{ScopeWrite}, AllJobs: true}); err != nil {
		t.Errorf("full access: checkWithin() = %v, want nil", err)
	}
}

package review

import (
	"strings"
	"testing"
)

func TestRelevantRules(t *testing.T) {
	rules := []Rule{
		{ID: "global", Description: "applies everywhere"},
		{ID: "go-only", Description: "go files", Paths: []string{"**/*.go"}},
		{ID: "api", Description: "api dir", Paths: []string{"apps/api/**"}},
		{ID: "not-gen", Description: "go but not generated", Paths: []string{"**/*.go"}, ExcludePaths: []string{"**/*.gen.go"}},
	}

	tests := []struct {
		name  string
		files []string
		want  []string // rule IDs expected, in order
	}{
		{"go file", []string{"internal/x.go"}, []string{"global", "go-only", "not-gen"}},
		{"api go file", []string{"apps/api/h.go"}, []string{"global", "go-only", "api", "not-gen"}},
		{"non-go file", []string{"README.md"}, []string{"global"}},
		{"only generated go", []string{"internal/x.gen.go"}, []string{"global", "go-only"}},
		{"no files", nil, []string{"global"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := relevantRules(rules, tt.files)
			var gotIDs []string
			for _, r := range got {
				gotIDs = append(gotIDs, r.ID)
			}
			if strings.Join(gotIDs, ",") != strings.Join(tt.want, ",") {
				t.Errorf("relevantRules(%v) = %v, want %v", tt.files, gotIDs, tt.want)
			}
		})
	}
}

func TestBuildPrompt_ScopesRules(t *testing.T) {
	cfg := &ReviewConfig{
		Rules: []Rule{
			{ID: "go-rule", Severity: "warning", Description: "Go convention", Paths: []string{"**/*.go"}},
			{ID: "ts-rule", Severity: "warning", Description: "TS convention", Paths: []string{"**/*.ts"}},
		},
	}
	pr := &PRData{Number: 1, Files: []string{"main.go"}}

	prompt := BuildPrompt(pr, cfg, 0, nil)

	if !strings.Contains(prompt, "go-rule") {
		t.Errorf("expected matching rule 'go-rule' in prompt")
	}
	if strings.Contains(prompt, "ts-rule") {
		t.Errorf("non-matching rule 'ts-rule' should be omitted from prompt")
	}
}

func TestBuildPrompt_NoMatchingRulesFallback(t *testing.T) {
	cfg := &ReviewConfig{
		Rules: []Rule{{ID: "ts-rule", Severity: "warning", Description: "TS", Paths: []string{"**/*.ts"}}},
	}
	pr := &PRData{Number: 1, Files: []string{"main.go"}}

	prompt := BuildPrompt(pr, cfg, 0, nil)
	if !strings.Contains(prompt, "No specific rules are defined") {
		t.Errorf("expected general-review fallback when no rule matches the changed files")
	}
}

package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRuleFile creates .claude/rules/<name> with the given content, relative
// to the current working directory (tests chdir into a temp dir first).
func writeRuleFile(t *testing.T, name, content string) {
	t.Helper()
	dir := filepath.Join(".claude", "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir rules dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writing rule file: %v", err)
	}
}

func TestSplitFrontmatter(t *testing.T) {
	meta, body := splitFrontmatter("---\ndescription: API rules\npaths:\n  - \"apps/api/**\"\n---\nUse the auth middleware.\n")
	if meta.Description != "API rules" {
		t.Errorf("description = %q, want %q", meta.Description, "API rules")
	}
	if len(meta.Paths) != 1 || meta.Paths[0] != "apps/api/**" {
		t.Errorf("paths = %v, want [apps/api/**]", meta.Paths)
	}
	if strings.TrimSpace(body) != "Use the auth middleware." {
		t.Errorf("body = %q, want %q", body, "Use the auth middleware.")
	}
}

func TestSplitFrontmatter_NoFrontmatter(t *testing.T) {
	meta, body := splitFrontmatter("Just a plain rule body.\n")
	if meta.Description != "" || len(meta.Paths) != 0 {
		t.Errorf("expected empty meta, got %+v", meta)
	}
	if body != "Just a plain rule body.\n" {
		t.Errorf("body = %q, want whole content", body)
	}
}

func TestSplitFrontmatter_Unterminated(t *testing.T) {
	content := "---\ndescription: oops\nno closing fence"
	meta, body := splitFrontmatter(content)
	if meta.Description != "" {
		t.Errorf("expected no meta for unterminated frontmatter, got %+v", meta)
	}
	if body != content {
		t.Errorf("expected whole content as body, got %q", body)
	}
}

func TestReadClaudeRules_ScopeIn(t *testing.T) {
	t.Chdir(t.TempDir())
	writeRuleFile(t, "api.md", "---\ndescription: API rules\npaths:\n  - \"apps/api/**\"\n---\nUse the auth middleware.\n")

	rules := ReadClaudeRules([]string{"apps/api/handler.go"})

	key := filepath.Join(".claude", "rules", "api.md")
	content, ok := rules[key]
	if !ok {
		t.Fatalf("expected rule %q to be included, got keys %v", key, keysOf(rules))
	}
	if !strings.Contains(content, "API rules") || !strings.Contains(content, "auth middleware") {
		t.Errorf("content missing description or body: %q", content)
	}
}

func TestReadClaudeRules_ScopeOut(t *testing.T) {
	t.Chdir(t.TempDir())
	writeRuleFile(t, "api.md", "---\npaths:\n  - \"apps/api/**\"\n---\nUse the auth middleware.\n")

	rules := ReadClaudeRules([]string{"apps/web/page.tsx"})
	if len(rules) != 0 {
		t.Errorf("expected no rules for non-matching change, got %v", keysOf(rules))
	}
}

func TestReadClaudeRules_NoPathsAlwaysIncluded(t *testing.T) {
	t.Chdir(t.TempDir())
	writeRuleFile(t, "global.md", "---\ndescription: global rule\n---\nAlways applies.\n")

	rules := ReadClaudeRules([]string{"anything/at/all.go"})
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule (no paths => always), got %v", keysOf(rules))
	}
}

func TestReadClaudeRules_NoFrontmatterAlwaysIncluded(t *testing.T) {
	t.Chdir(t.TempDir())
	writeRuleFile(t, "plain.md", "No frontmatter here, just conventions.\n")

	rules := ReadClaudeRules([]string{"src/x.go"})
	if len(rules) != 1 {
		t.Fatalf("expected plain rule to always be included, got %v", keysOf(rules))
	}
}

func TestReadClaudeRules_PerFileTruncation(t *testing.T) {
	t.Chdir(t.TempDir())
	big := strings.Repeat("x", maxRuleBytes+500)
	writeRuleFile(t, "big.md", big)

	rules := ReadClaudeRules([]string{"src/x.go"})
	key := filepath.Join(".claude", "rules", "big.md")
	content := rules[key]
	if !strings.Contains(content, "... (truncated)") {
		t.Errorf("expected truncation marker in oversized rule")
	}
	if len(content) > maxRuleBytes+len("\n... (truncated)") {
		t.Errorf("content not truncated: %d bytes", len(content))
	}
}

func TestReadClaudeRules_TotalBudget(t *testing.T) {
	t.Chdir(t.TempDir())
	// Each near maxRuleBytes; total budget admits some but not all.
	chunk := strings.Repeat("y", maxRuleBytes-100)
	for _, n := range []string{"a.md", "b.md", "c.md", "d.md", "e.md", "f.md"} {
		writeRuleFile(t, n, chunk)
	}

	rules := ReadClaudeRules([]string{"src/x.go"})
	total := 0
	for _, c := range rules {
		total += len(c)
	}
	if total > maxTotalRuleBytes {
		t.Errorf("total rule bytes %d exceeds budget %d", total, maxTotalRuleBytes)
	}
	if len(rules) == 0 {
		t.Errorf("expected at least one rule within budget")
	}
}

// writeDocFile creates <path> (relative to cwd), making parent dirs.
func writeDocFile(t *testing.T, path, content string) {
	t.Helper()
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestReadProjectDocs_RootAndClaudeAlways(t *testing.T) {
	t.Chdir(t.TempDir())
	writeDocFile(t, "CLAUDE.md", "root conventions")
	writeDocFile(t, ".claude/CLAUDE.md", "claude conventions")

	docs := ReadProjectDocs(nil) // no changed files
	if _, ok := docs["CLAUDE.md"]; !ok {
		t.Errorf("root CLAUDE.md should always be included")
	}
	if _, ok := docs[".claude/CLAUDE.md"]; !ok {
		t.Errorf(".claude/CLAUDE.md should always be included")
	}
}

func TestReadProjectDocs_NestedScopedIn(t *testing.T) {
	t.Chdir(t.TempDir())
	writeDocFile(t, "apps/web/CLAUDE.md", "web conventions")
	writeDocFile(t, "engines/billing/CLAUDE.md", "billing conventions")

	docs := ReadProjectDocs([]string{"apps/web/src/page.tsx"})

	if _, ok := docs[filepath.Join("apps", "web", "CLAUDE.md")]; !ok {
		t.Errorf("nested apps/web/CLAUDE.md should be included when a changed file is under it; got %v", keysOf(docs))
	}
	if _, ok := docs[filepath.Join("engines", "billing", "CLAUDE.md")]; ok {
		t.Errorf("engines/billing/CLAUDE.md should be excluded — no changed file under it")
	}
}

func TestReadProjectDocs_SkipDirsNotWalked(t *testing.T) {
	t.Chdir(t.TempDir())
	writeDocFile(t, "node_modules/pkg/CLAUDE.md", "should never load")

	docs := ReadProjectDocs([]string{"node_modules/pkg/index.js"})
	if _, ok := docs[filepath.Join("node_modules", "pkg", "CLAUDE.md")]; ok {
		t.Errorf("CLAUDE.md under node_modules must not be discovered")
	}
}

func TestReadProjectDocs_LargeRootNotTruncatedUnderCap(t *testing.T) {
	t.Chdir(t.TempDir())
	// 36KB root doc — used to be truncated at 12KB; now fits under maxDocBytes.
	big := strings.Repeat("a", 36*1024)
	writeDocFile(t, "CLAUDE.md", big)

	docs := ReadProjectDocs(nil)
	content := docs["CLAUDE.md"]
	if strings.Contains(content, "... (truncated)") {
		t.Errorf("36KB root doc should not be truncated under the raised cap")
	}
	if len(content) != len(big) {
		t.Errorf("expected full %d bytes, got %d", len(big), len(content))
	}
}

func TestReadProjectDocs_TruncatesAboveCap(t *testing.T) {
	t.Chdir(t.TempDir())
	writeDocFile(t, "CLAUDE.md", strings.Repeat("a", maxDocBytes+100))

	docs := ReadProjectDocs(nil)
	if !strings.Contains(docs["CLAUDE.md"], "... (truncated)") {
		t.Errorf("doc above maxDocBytes should be truncated")
	}
}

func keysOf(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

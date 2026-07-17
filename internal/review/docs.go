package review

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// skipDirs are directories to ignore when walking the repo.
var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, ".git": true,
	"dist": true, "build": true, ".next": true, ".nuxt": true,
	"target": true, "__pycache__": true, ".venv": true, "venv": true,
	".claude": true, ".codecanary": true,
}

// maxDocBytes caps the size of a single CLAUDE.md file included in the prompt.
// Sized to admit a large monorepo root doc (tens of KB) without truncation.
const maxDocBytes = 65536

// maxTotalDocBytes caps the total size of all CLAUDE.md files combined.
const maxTotalDocBytes = 131072

// maxDocFiles caps the number of CLAUDE.md files included. Nested docs are
// path-scoped to the change, so this only bounds pathological repos.
const maxDocFiles = 20

// ReadProjectDocs reads CLAUDE.md files relevant to the changed files. The root
// CLAUDE.md and .claude/CLAUDE.md are always included; nested <dir>/CLAUDE.md
// files (at any depth) are included only when a changed file lives under that
// directory. It returns a map of path → content, respecting per-file, total,
// and file-count limits.
func ReadProjectDocs(changedFiles []string) map[string]string {
	docs := make(map[string]string)

	// Always include the root and .claude docs.
	docPaths := []string{"CLAUDE.md", ".claude/CLAUDE.md"}

	// Discover nested CLAUDE.md files, scoped to the change.
	docPaths = append(docPaths, discoverScopedDocs(changedFiles)...)

	totalBytes := 0
	for _, p := range docPaths {
		if len(docs) >= maxDocFiles {
			break
		}
		if _, seen := docs[p]; seen {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := string(data)
		if len(content) > maxDocBytes {
			content = content[:maxDocBytes] + "\n... (truncated)"
		}
		if totalBytes+len(content) > maxTotalDocBytes {
			continue
		}
		docs[p] = content
		totalBytes += len(content)
	}

	return docs
}

// discoverScopedDocs walks the repo for nested CLAUDE.md files and returns those
// whose directory contains at least one changed file. The root CLAUDE.md and
// anything under skipDirs / dot-prefixed directories are excluded (the root and
// .claude docs are handled by the caller).
func discoverScopedDocs(changedFiles []string) []string {
	if len(changedFiles) == 0 {
		return nil
	}

	var found []string
	_ = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != "." && (skipDirs[name] || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "CLAUDE.md" {
			return nil
		}
		dir := filepath.Dir(path)
		if dir == "." {
			return nil // root CLAUDE.md handled by caller
		}
		// Include only when a changed file lives under this directory.
		if anyFileMatches(changedFiles, []string{dir + "/**"}) {
			found = append(found, path)
		}
		return nil
	})
	return found
}

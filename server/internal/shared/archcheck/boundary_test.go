package archcheck

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoundary_HubDoesNotImportRegistry(t *testing.T) {
	checkNoImport(t,
		filepath.FromSlash("internal/hub"),
		"\"github.com/swm8023/wheelmaker/internal/registry\"",
	)
}

func TestBoundary_RegistryDoesNotImportHub(t *testing.T) {
	checkNoImport(t,
		filepath.FromSlash("internal/registry"),
		"\"github.com/swm8023/wheelmaker/internal/hub\"",
	)
}

func checkNoImport(t *testing.T, root, forbidden string) {
	t.Helper()
	repoRoot := findRepoRoot(t)
	absRoot := filepath.Join(repoRoot, root)
	err := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range f.Imports {
			if imp.Path != nil && imp.Path.Value == forbidden {
				rel, _ := filepath.Rel(repoRoot, path)
				t.Fatalf("boundary violation: %s imports %s", filepath.ToSlash(rel), forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// file is under server/internal/shared/archcheck; go up to server root.
	return filepath.Clean(filepath.Join(wd, "..", "..", ".."))
}

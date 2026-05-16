package history

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageCompiles(t *testing.T) {
	t.Parallel()
}

func TestHistoryPackageBoundaryExcludesLaterMilestoneBehavior(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read history package dir: %v", err)
	}

	forbiddenImports := map[string]bool{
		"os":                                    true,
		"os/exec":                               true,
		"github.com/jgabor/aila/internal/agent": true,
		"github.com/jgabor/aila/internal/context": true,
		"github.com/jgabor/aila/internal/tools":   true,
		"github.com/jgabor/aila/internal/tui":     true,
	}
	forbiddenTokens := []string{
		"undo",
		"redo",
		"mutation",
		"real model",
		"real tool",
		"replay",
		"compaction",
		"read tool",
		"find tool",
		"grep tool",
		"bash tool",
		"plugin",
		"MCP",
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(".", name)
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s imports: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, "\"")
			if forbiddenImports[importPath] {
				t.Fatalf("history source %s imports forbidden package %q", path, importPath)
			}
		}
		for _, token := range forbiddenTokens {
			if strings.Contains(string(source), token) {
				t.Fatalf("history source %s contains forbidden boundary token %q", path, token)
			}
		}
	}
}

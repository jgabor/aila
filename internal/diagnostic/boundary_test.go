package diagnostic

import (
	"go/ast"
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

func TestDiagnosticPackageBoundaryStaysPassive(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read diagnostic package dir: %v", err)
	}

	forbiddenImports := map[string]bool{
		"os/exec":                               true,
		"net":                                   true,
		"net/http":                              true,
		"github.com/jgabor/aila/internal/agent": true,
		"github.com/jgabor/aila/internal/app":   true,
		"github.com/jgabor/aila/internal/capability": true,
		"github.com/jgabor/aila/internal/permission": true,
		"github.com/jgabor/aila/internal/policy":     true,
		"github.com/jgabor/aila/internal/runtime":    true,
		"github.com/jgabor/aila/internal/state":      true,
		"github.com/jgabor/aila/internal/tools":      true,
		"github.com/jgabor/aila/internal/tui":        true,
		"github.com/jgabor/aila/internal/workflow":   true,
	}
	forbiddenTokens := []string{
		"exec.Command",
		"tea.",
		"bubbletea",
		"Transition(",
		"Update(",
		"ModelCall",
		"go-agent",
		"tool execution",
		"Approve(",
		"Retry(",
		"Fallback(",
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
				t.Fatalf("diagnostic source %s imports forbidden package %q", path, importPath)
			}
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Command" {
				t.Fatalf("diagnostic source %s performs command execution", path)
			}
			return true
		})

		for _, token := range forbiddenTokens {
			if strings.Contains(string(source), token) {
				t.Fatalf("diagnostic source %s contains forbidden boundary token %q", path, token)
			}
		}
	}
}

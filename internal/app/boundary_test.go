package app

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestPackageCompiles(t *testing.T) {
	t.Parallel()
}

func TestStartupPhaseWiringBoundaryStaysDisplayOnly(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "app.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse app startup boundary: %v", err)
	}

	imports := map[string]bool{}
	for _, spec := range parsed.Imports {
		imports[strings.Trim(spec.Path.Value, "\"")] = true
	}
	for _, forbidden := range []string{
		"os/exec",
		"net",
		"net/http",
		"github.com/jgabor/aila/internal/capability",
		"github.com/jgabor/aila/internal/context",
		"github.com/jgabor/aila/internal/history",
		"github.com/jgabor/aila/internal/permission",
		"github.com/jgabor/aila/internal/policy",
		"github.com/jgabor/aila/internal/runtime",
		"github.com/jgabor/aila/internal/tools",
		"github.com/jgabor/aila/internal/utility",
	} {
		if imports[forbidden] {
			t.Fatalf("startup imports forbidden future-scope package %q", forbidden)
		}
	}

	source, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatalf("read app startup source: %v", err)
	}
	for _, forbidden := range []string{
		"Transition",
		"transition",
		"FSM",
		"Runtime",
		"runtime.",
		"Replay",
		"Compaction",
		"History",
		"Index",
		"Adapter",
		"interface",
		"go func",
		"for {",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("startup wiring contains future-scope behavior token %q", forbidden)
		}
	}
}

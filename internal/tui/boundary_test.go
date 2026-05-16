package tui

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestPackageCompiles(t *testing.T) {
	t.Parallel()
}

func TestTUIBoundaryDoesNotImportStateOrHistoryPersistence(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"model.go", "render.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports for %s: %v", path, err)
		}
		for _, imported := range file.Imports {
			pathValue := strings.Trim(imported.Path.Value, "\"")
			for _, forbidden := range []string{
				"github.com/jgabor/aila/internal/state",
				"github.com/jgabor/aila/internal/history",
			} {
				if pathValue == forbidden {
					t.Fatalf("%s imports %s; state, memory, and history must be injected as display fields", path, forbidden)
				}
			}
		}
	}
}

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

func TestTUIBoundaryDoesNotImportStateForResumeMemory(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"model.go", "render.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports for %s: %v", path, err)
		}
		for _, imported := range file.Imports {
			if strings.Trim(imported.Path.Value, "\"") == "github.com/jgabor/aila/internal/state" {
				t.Fatalf("%s imports internal/state; resume memory must be injected as display fields", path)
			}
		}
	}
}

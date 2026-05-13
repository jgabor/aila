package workflow

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestPhasesHaveStableIdentifiersAndLabels(t *testing.T) {
	t.Parallel()

	want := []struct {
		phase Phase
		id    string
		label string
	}{
		{PhaseIdle, "idle", "IDLE"},
		{PhaseEnvision, "envision", "ENVISION"},
		{PhaseDeliberate, "deliberate", "DELIBERATE"},
		{PhasePlan, "plan", "PLAN"},
		{PhaseBuild, "build", "BUILD"},
		{PhaseAudit, "audit", "AUDIT"},
	}

	got := Phases()
	if len(got) != len(want) {
		t.Fatalf("len(Phases()) = %d, want %d", len(got), len(want))
	}
	for i, expected := range want {
		if got[i] != expected.phase {
			t.Fatalf("Phases()[%d] = %q, want %q", i, got[i], expected.phase)
		}
		if string(expected.phase) != expected.id {
			t.Fatalf("%s identifier = %q, want %q", expected.label, string(expected.phase), expected.id)
		}
		if expected.phase.DisplayLabel() != expected.label {
			t.Fatalf("%s DisplayLabel() = %q, want %q", expected.id, expected.phase.DisplayLabel(), expected.label)
		}
	}
}

func TestPhasesReturnsCopy(t *testing.T) {
	t.Parallel()

	got := Phases()
	got[0] = PhaseAudit
	if Phases()[0] != PhaseIdle {
		t.Fatalf("Phases() returned mutable package storage")
	}
}

func TestPhaseStringAndFormatUseStableIdentifier(t *testing.T) {
	t.Parallel()

	phase := PhaseBuild
	if phase.String() != "build" {
		t.Fatalf("String() = %q, want build", phase.String())
	}
	if fmt.Sprint(phase) != "build" {
		t.Fatalf("fmt.Sprint(phase) = %q, want build", fmt.Sprint(phase))
	}
	if fmt.Sprintf("%q", phase) != "\"build\"" {
		t.Fatalf("fmt %%q = %q, want quoted build", fmt.Sprintf("%q", phase))
	}
}

func TestParsePhaseAcceptsStableIdentifiers(t *testing.T) {
	t.Parallel()

	for _, phase := range Phases() {
		got, err := ParsePhase(phase.String())
		if err != nil {
			t.Fatalf("ParsePhase(%q) returned error: %v", phase, err)
		}
		if got != phase {
			t.Fatalf("ParsePhase(%q) = %q, want %q", phase, got, phase)
		}
	}
}

func TestParsePhaseNormalizesCaseAndWhitespace(t *testing.T) {
	t.Parallel()

	got, err := ParsePhase("  BUILD\n")
	if err != nil {
		t.Fatalf("ParsePhase returned error: %v", err)
	}
	if got != PhaseBuild {
		t.Fatalf("ParsePhase = %q, want %q", got, PhaseBuild)
	}
}

func TestParsePhaseInvalidErrorIsBounded(t *testing.T) {
	t.Parallel()

	invalid := strings.Repeat("x", 4096)
	phase, err := ParsePhase(invalid)
	if err == nil {
		t.Fatal("ParsePhase returned nil error for invalid phase")
	}
	if phase != "" {
		t.Fatalf("ParsePhase invalid phase = %q, want empty", phase)
	}
	message := err.Error()
	if !strings.HasPrefix(message, "invalid workflow phase ") {
		t.Fatalf("error = %q, want invalid workflow phase prefix", message)
	}
	if len(message) > 120 {
		t.Fatalf("error length = %d, want bounded <= 120: %q", len(message), message)
	}
	if strings.Contains(message, "successor") || strings.Contains(message, "transition") {
		t.Fatalf("invalid parse error implies transition semantics: %q", message)
	}
}

func TestWorkflowPackageDefinesNoTransitionSurface(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob workflow files: %v", err)
	}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || file == "doc.go" {
			continue
		}

		set := token.NewFileSet()
		parsed, err := parser.ParseFile(set, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}

		ast.Inspect(parsed, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.FuncDecl:
				assertNoTransitionName(t, file, n.Name.Name)
			case *ast.TypeSpec:
				assertNoTransitionName(t, file, n.Name.Name)
			case *ast.ValueSpec:
				for _, name := range n.Names {
					assertNoTransitionName(t, file, name.Name)
				}
			}
			return true
		})
	}
}

func assertNoTransitionName(t *testing.T, file, name string) {
	t.Helper()

	lower := strings.ToLower(name)
	if strings.Contains(lower, "transition") || strings.Contains(lower, "successor") {
		t.Fatalf("%s defines transition surface %q during vocabulary-only task", file, name)
	}
}

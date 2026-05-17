package context

import (
	"strconv"
	"strings"
	"testing"
)

func TestBuilderPreservesSourceRefsForPromptToolsDiffCommandsAndConstraints(t *testing.T) {
	t.Parallel()

	built := Build(BuildInput{
		Prompts: []PromptInput{{Text: "explain the current diff"}},
		ToolResults: []ToolResultInput{{
			ToolName:   "read",
			Status:     "completed",
			Summary:    "README.md describes context state",
			ExactLines: []string{"README.md:125 .aila is project state"},
			SourceRefs: []SourceRef{{ID: "readme-context", Kind: SourceToolResult, Path: "README.md", LineStart: 125, LineEnd: 125, Excerpt: ".aila is project state"}},
		}},
		Diffs: []DiffInput{{
			Path:       "internal/app/session_snapshot.go",
			Summary:    "session snapshot carries source refs",
			HunkLines:  []string{"+ SourceRefs []string"},
			SourceRefs: []SourceRef{{ID: "diff-session", Kind: SourceDiff, Path: "internal/app/session_snapshot.go", Excerpt: "+ SourceRefs []string"}},
		}},
		Commands: []CommandOutputInput{{
			Command:     "git status --short",
			Status:      "completed",
			ExitCode:    0,
			StdoutLines: []string{"M internal/context/context.go"},
		}},
		UserConstraints: []UserConstraintInput{{Text: "Do not add /compact in M39."}},
		MaxBytes:        128,
	})

	if len(built.Blocks) != 5 || len(built.SourceRefs) < 5 || len(built.Claims) < 4 {
		t.Fatalf("built context counts = blocks:%d refs:%d claims:%d", len(built.Blocks), len(built.SourceRefs), len(built.Claims))
	}
	if built.Budget.BlockCount != len(built.Blocks) || built.Budget.SourceRefCount != len(built.SourceRefs) || !built.Budget.Truncated {
		t.Fatalf("budget = %+v, want counts and truncation marker", built.Budget)
	}
	assertHasRef(t, built, "readme-context", SourceToolResult, "README.md", ".aila is project state")
	assertHasRef(t, built, "diff-session", SourceDiff, "internal/app/session_snapshot.go", "+ SourceRefs []string")
	assertHasRef(t, built, "command-1-stdout-1", SourceCommandStdout, "", "M internal/context/context.go")
	assertHasRef(t, built, "constraint-5", SourceUserConstraint, "", "Do not add /compact in M39.")
	assertHasClaimWithRef(t, built, "command git status --short completed exit 0", "command-1-stdout-1")
	if !strings.Contains(built.MeterLabel(), "5 blocks") || !strings.Contains(built.MeterLabel(), "refs") || !strings.Contains(built.MeterLabel(), "truncated") {
		t.Fatalf("meter label = %q", built.MeterLabel())
	}
}

func TestBuilderNormalizesDuplicateAndEmptySourceRefs(t *testing.T) {
	t.Parallel()

	built := Build(BuildInput{ToolResults: []ToolResultInput{{
		ToolName: "grep",
		Status:   "completed",
		Summary:  "two refs share one incoming id",
		SourceRefs: []SourceRef{
			{ID: "shared ref", Kind: SourceToolResult, Path: "a.go", Excerpt: "first"},
			{ID: "shared ref", Kind: SourceToolResult, Path: "b.go", Excerpt: "second"},
			{Kind: SourceToolResult, Path: "c.go", Excerpt: "third"},
		},
	}}})

	assertHasRef(t, built, "shared-ref", SourceToolResult, "a.go", "first")
	assertHasRef(t, built, "shared-ref-2", SourceToolResult, "b.go", "second")
	assertHasRef(t, built, "source-3", SourceToolResult, "c.go", "third")
	if got := built.Blocks[0].SourceRefIDs; len(got) != 3 || got[0] != "shared-ref" || got[1] != "shared-ref-2" || got[2] != "source-3" {
		t.Fatalf("block refs = %#v, want normalized unique refs", got)
	}
}

func TestCommandContextPreservesExactFailureLines(t *testing.T) {
	t.Parallel()

	built := Build(BuildInput{Commands: []CommandOutputInput{{
		Command:      "git checkout main",
		Status:       "failed",
		ExitCode:     -1,
		StderrLines:  []string{"git subcommand is not allowed", "exact failure line: git checkout main"},
		ErrorKind:    "unsafe_command",
		ErrorMessage: "git subcommand is not allowed",
	}}})

	if len(built.Blocks) != 1 || !strings.Contains(built.Blocks[0].Text, "exact failure line: git checkout main") {
		t.Fatalf("command block = %+v, want exact failure line", built.Blocks)
	}
	assertHasRef(t, built, "command-1-stderr-2", SourceCommandStderr, "", "exact failure line: git checkout main")
	assertHasRef(t, built, "command-1-failure", SourceCommandFailure, "", "git subcommand is not allowed")
	assertHasClaimWithRef(t, built, "command git checkout main failed: unsafe_command: git subcommand is not allowed", "command-1-failure")
}

func TestCompactPreservesSourceRefsAndExactCriticalDetails(t *testing.T) {
	t.Parallel()

	built := Build(BuildInput{
		Prompts: []PromptInput{{Text: "review the compact command"}},
		ToolResults: []ToolResultInput{{
			ToolName: "read",
			Status:   "completed",
			Summary:  "ROADMAP.md names manual compact",
			SourceRefs: []SourceRef{{
				ID:        "roadmap-compact",
				Kind:      SourceToolResult,
				Path:      "ROADMAP.md",
				LineStart: 1624,
				LineEnd:   1645,
				Excerpt:   "Manual compaction runs through explicit effects.",
			}},
		}},
		Diffs: []DiffInput{{
			Path:      "internal/context/context.go",
			Summary:   "compaction result carries source refs",
			HunkLines: []string{"+ CompactResult"},
		}},
		Commands: []CommandOutputInput{{
			Command:      "go test ./internal/context",
			Status:       "failed",
			ExitCode:     1,
			StderrLines:  []string{"exact failure: missing source ref"},
			ErrorKind:    "execution_error",
			ErrorMessage: "exact failure: missing source ref",
		}},
		UserConstraints: []UserConstraintInput{{Text: "Do not add background compaction."}},
	})

	compacted := Compact(CompactInput{Context: built, MaxBytes: 128})
	if compacted.OriginalBudget.BlockCount != built.Budget.BlockCount {
		t.Fatalf("original budget = %+v, want %d blocks", compacted.OriginalBudget, built.Budget.BlockCount)
	}
	if len(compacted.Context.Blocks) != 1 || len(compacted.Context.SourceRefs) != len(built.SourceRefs) {
		t.Fatalf("compacted counts = blocks:%d refs:%d, want one block and preserved refs:%d", len(compacted.Context.Blocks), len(compacted.Context.SourceRefs), len(built.SourceRefs))
	}
	text := compacted.Context.Blocks[0].Text
	for _, exact := range []string{
		"ROADMAP.md",
		"Manual compaction runs through explicit effects.",
		"go test ./internal/context",
		"exact failure: missing source ref",
		"Do not add background compaction.",
	} {
		if !strings.Contains(text, exact) {
			t.Fatalf("compacted text missing %q:\n%s", exact, text)
		}
	}
	assertHasRef(t, compacted.Context, "roadmap-compact", SourceToolResult, "ROADMAP.md", "Manual compaction runs through explicit effects.")
	assertHasRef(t, compacted.Context, "command-1-failure", SourceCommandFailure, "", "exact failure: missing source ref")
	assertHasClaimWithRef(t, compacted.Context, "manual compaction preserved "+strconv.Itoa(len(built.SourceRefs))+" source refs", "roadmap-compact")
	if !compacted.Context.Budget.Truncated || len(compacted.Caveats) == 0 {
		t.Fatalf("compacted budget/caveats = %+v %#v, want over-budget caveat", compacted.Context.Budget, compacted.Caveats)
	}
}

func TestCompactReportsEmptyContextCaveat(t *testing.T) {
	t.Parallel()

	compacted := Compact(CompactInput{MaxBytes: 80})
	if len(compacted.Context.Blocks) != 0 || len(compacted.Caveats) != 1 || !strings.Contains(compacted.Caveats[0], "no context") {
		t.Fatalf("empty compaction = blocks:%d caveats:%#v", len(compacted.Context.Blocks), compacted.Caveats)
	}
	if compacted.Context.Budget.MaxBytes != 80 || compacted.Context.Budget.SourceRefCount != 0 {
		t.Fatalf("empty budget = %+v", compacted.Context.Budget)
	}
}

func assertHasRef(t *testing.T, built BuiltContext, id string, kind SourceKind, path string, excerpt string) {
	t.Helper()
	for _, ref := range built.SourceRefs {
		if ref.ID == id {
			if ref.Kind != kind || ref.Path != path || ref.Excerpt != excerpt {
				t.Fatalf("ref %s = %+v, want kind=%s path=%q excerpt=%q", id, ref, kind, path, excerpt)
			}
			return
		}
	}
	t.Fatalf("missing source ref %q in %+v", id, built.SourceRefs)
}

func assertHasClaimWithRef(t *testing.T, built BuiltContext, text string, refID string) {
	t.Helper()
	for _, claim := range built.Claims {
		if claim.Text == text {
			for _, id := range claim.SourceRefIDs {
				if id == refID {
					return
				}
			}
			t.Fatalf("claim %q refs = %#v, missing %q", text, claim.SourceRefIDs, refID)
		}
	}
	t.Fatalf("missing claim %q in %+v", text, built.Claims)
}

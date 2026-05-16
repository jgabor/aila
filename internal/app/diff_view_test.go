package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

func TestDiffOpenUsesInjectedReaderWithoutRuntimeOrPersistence(t *testing.T) {
	t.Parallel()

	view := snapshotTestView()
	view.Phase = "BUILD"
	view.PhaseSource = "workflow.fixture"
	view.RuntimeStatus = "idle"
	view.RuntimeResult = "stable before diff"
	readCalls := 0
	controller := newSessionControllerWithPersistenceHistoryReadAndDiff(context.Background(), view, newInputRunnerWithDispatch(runtime.Dispatch), func(context.Context, SnapshotPersistenceCommand) SnapshotPersistenceResult {
		t.Fatal("diff open must not persist session snapshot")
		return SnapshotPersistenceResult{}
	}, func(context.Context, HistoryPersistenceCommand) HistoryPersistenceResult {
		t.Fatal("diff open must not persist fake history")
		return HistoryPersistenceResult{}
	}, func(context.Context, HistoryReadCommand) HistoryReadResult {
		t.Fatal("diff open must not read fake history")
		return HistoryReadResult{}
	}, func(context.Context, DiffReadCommand) DiffReadResult {
		readCalls++
		return DiffReadResult{View: &tui.DiffView{Source: "test.diff", Status: "ready", Files: []tui.DiffFileView{{Path: "internal/demo.txt", Status: "modified", Hunks: []tui.DiffHunkView{{Header: "@@ -1 +1 @@", OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: []tui.DiffLineView{{Kind: "removal", Text: "old", OldLine: 1}, {Kind: "addition", Text: "new", NewLine: 1}}}}}}}}
	})

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteDiff, Kind: policy.CommandInputSlash}, controller.view)

	if readCalls != 1 {
		t.Fatalf("diff read calls = %d, want 1", readCalls)
	}
	if got.Phase != view.Phase || got.PhaseSource != view.PhaseSource {
		t.Fatalf("diff open mutated workflow display from %q/%q to %q/%q", view.Phase, view.PhaseSource, got.Phase, got.PhaseSource)
	}
	if got.RuntimeStatus != view.RuntimeStatus || got.RuntimeResult != view.RuntimeResult {
		t.Fatalf("diff open mutated runtime display: before=%+v after=%+v", view, got)
	}
	if got.SurfaceTitle != "diff" || got.CommandRoute != "diff" || !got.DiffFocus || got.Diff == nil || len(got.Diff.Files) != 1 {
		t.Fatalf("diff view state = %+v, want focused one-file read-only diff", got)
	}
	if got.Diff.Source != "test.diff" || got.Diff.Files[0].Path != "internal/demo.txt" || got.Diff.Files[0].Hunks[0].Lines[1].Kind != "addition" {
		t.Fatalf("diff view = %+v, want injected exact path and addition", got.Diff)
	}
}

func TestParseUnifiedDiffViewPreservesPathsHunksAndLineKinds(t *testing.T) {
	t.Parallel()

	view := parseUnifiedDiffView("fixture.diff", strings.Join([]string{
		"diff --git a/internal/demo.txt b/internal/demo.txt",
		"index 1111111..2222222 100644",
		"--- a/internal/demo.txt",
		"+++ b/internal/demo.txt",
		"@@ -2,2 +2,3 @@",
		" context",
		"-old value",
		"+new value",
		"+second value",
	}, "\n"))

	if view == nil || view.Status != "ready" || view.Source != "fixture.diff" || len(view.Files) != 1 {
		t.Fatalf("parsed diff = %+v, want one ready file", view)
	}
	file := view.Files[0]
	if file.Path != "internal/demo.txt" || file.OldPath != "internal/demo.txt" || file.Status != "modified" || len(file.Hunks) != 1 {
		t.Fatalf("parsed file = %+v, want normalized path and one hunk", file)
	}
	hunk := file.Hunks[0]
	if hunk.OldStart != 2 || hunk.OldLines != 2 || hunk.NewStart != 2 || hunk.NewLines != 3 || len(hunk.Lines) != 4 {
		t.Fatalf("parsed hunk = %+v, want header counts and four lines", hunk)
	}
	wantKinds := []string{"context", "removal", "addition", "addition"}
	for index, want := range wantKinds {
		if hunk.Lines[index].Kind != want {
			t.Fatalf("line %d kind = %q, want %q in %+v", index, hunk.Lines[index].Kind, want, hunk.Lines)
		}
	}
	if hunk.Lines[1].OldLine != 3 || hunk.Lines[2].NewLine != 3 {
		t.Fatalf("line numbers = %+v, want removal old line 3 and addition new line 3", hunk.Lines)
	}
}

func TestStoreCurrentDiffReadUsesReadOnlyGitDiff(t *testing.T) {
	workspace := t.TempDir()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "internal"), 0o755); err != nil {
		t.Fatalf("create fixture dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "demo.txt"), []byte("old value\n"), 0o644); err != nil {
		t.Fatalf("write base file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "staged.txt"), []byte("staged old\n"), 0o644); err != nil {
		t.Fatalf("write staged base file: %v", err)
	}
	runGit(t, workspace, "init")
	runGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "add", "internal/demo.txt", "internal/staged.txt")
	runGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
	if err := os.WriteFile(filepath.Join(workspace, "internal", "staged.txt"), []byte("staged new\n"), 0o644); err != nil {
		t.Fatalf("write staged changed file: %v", err)
	}
	runGit(t, workspace, "add", "internal/staged.txt")
	if err := os.WriteFile(filepath.Join(workspace, "internal", "demo.txt"), []byte("new value\nsecond value\n"), 0o644); err != nil {
		t.Fatalf("write changed file: %v", err)
	}

	result := storeCurrentDiffRead(workspace)(context.Background(), DiffReadCommand{})

	if result.View == nil || result.View.Status != "ready" || result.View.Source != "git diff" || len(result.View.Files) != 2 {
		t.Fatalf("current diff result = %+v, want staged and unstaged git diff files", result.View)
	}
	files := map[string]tui.DiffFileView{}
	for _, file := range result.View.Files {
		files[file.Path] = file
	}
	demo := files["internal/demo.txt"]
	if demo.Path != "internal/demo.txt" || demo.Status != "modified" || len(demo.Hunks) != 1 {
		t.Fatalf("current diff demo file = %+v, want modified internal/demo.txt", demo)
	}
	demoLines := demo.Hunks[0].Lines
	if len(demoLines) != 3 || demoLines[0].Kind != "removal" || demoLines[0].Text != "old value" || demoLines[1].Kind != "addition" || demoLines[1].Text != "new value" || demoLines[2].Kind != "addition" || demoLines[2].Text != "second value" {
		t.Fatalf("current diff demo lines = %+v, want one removal and two additions", demoLines)
	}
	staged := files["internal/staged.txt"]
	if staged.Path != "internal/staged.txt" || staged.Status != "modified" || len(staged.Hunks) != 1 {
		t.Fatalf("current diff staged file = %+v, want modified internal/staged.txt", staged)
	}
	stagedLines := staged.Hunks[0].Lines
	if len(stagedLines) != 2 || stagedLines[0].Kind != "removal" || stagedLines[0].Text != "staged old" || stagedLines[1].Kind != "addition" || stagedLines[1].Text != "staged new" {
		t.Fatalf("current diff staged lines = %+v, want staged removal and addition", stagedLines)
	}
}

func runGit(t *testing.T, workspace string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = workspace
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

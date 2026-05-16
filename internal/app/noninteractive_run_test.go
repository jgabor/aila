package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/state"
)

func TestNonInteractiveRunInspectsRepoAndStoresSessionMemory(t *testing.T) {
	t.Parallel()

	workspace := seedNonInteractiveRunWorkspace(t)
	output, err := NonInteractiveRunCommandOutput(context.Background(), NonInteractiveRunRequest{
		Version:       "test-version",
		Prompt:        "explain the repo",
		WorkspacePath: workspace,
	})
	if err != nil {
		t.Fatalf("NonInteractiveRunCommandOutput returned error: %v", err)
	}
	for _, want := range []string{
		"aila test-version",
		"command: run",
		"mode: non_interactive_read_only",
		"status: flagged",
		"prompt: explain the repo",
		"inspected_files:",
		"- README.md status=completed source_ref=README.md:1-",
		"- ROADMAP.md status=completed source_ref=ROADMAP.md:1-",
		"commands_run:",
		"- git status --short --branch status=completed exit=0 summary=",
		"- git diff --stat status=completed exit=0 summary=",
		"caveats:",
		"deterministic read-only run; provider model execution deferred",
		"source_refs:",
		"stored_session: true",
		"stored_history: true",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, workspace) {
		t.Fatalf("output leaked absolute workspace path %q:\n%s", workspace, output)
	}

	store := mustOpenProjectStoreForRun(t, workspace)
	snapshot, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("ReadCurrentSessionSnapshot returned error: %v", err)
	}
	if snapshot.State != state.SessionSnapshotLoaded || snapshot.Snapshot.Run == nil {
		t.Fatalf("snapshot state = %q run=%#v, want loaded run memory", snapshot.State, snapshot.Snapshot.Run)
	}
	run := snapshot.Snapshot.Run
	if run.Mode != "non_interactive_read_only" || run.Prompt != "explain the repo" || run.Status != "flagged" {
		t.Fatalf("snapshot run identity = mode=%q prompt=%q status=%q", run.Mode, run.Prompt, run.Status)
	}
	if !run.StoredSession || !run.StoredHistory {
		t.Fatalf("snapshot stored flags = session=%v history=%v, want both true", run.StoredSession, run.StoredHistory)
	}
	if len(run.InspectedFiles) < 2 || len(run.Commands) != 2 || len(run.SourceRefs) < 4 {
		t.Fatalf("snapshot run evidence = files=%#v commands=%#v source_refs=%#v", run.InspectedFiles, run.Commands, run.SourceRefs)
	}
	if !containsText(run.Caveats, "provider model execution deferred") {
		t.Fatalf("snapshot caveats = %#v, want provider deferral caveat", run.Caveats)
	}
	if snapshotText := strings.Join([]string{run.Prompt, run.InspectedFiles[0].Path, run.Commands[0].Command, strings.Join(run.SourceRefs, "\n")}, "\n"); strings.Contains(snapshotText, workspace) {
		t.Fatalf("snapshot run memory leaked absolute workspace path %q in %q", workspace, snapshotText)
	}

	history, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("ReadFakeHistory returned error: %v", err)
	}
	if history.State != state.FakeHistoryLoaded || len(history.Events) < 5 {
		t.Fatalf("history state = %q events=%d, want loaded prompt/response/runtime/commands", history.State, len(history.Events))
	}
	joinedHistory := make([]string, 0, len(history.Events))
	for _, event := range history.Events {
		joinedHistory = append(joinedHistory, event.DisplayText)
	}
	joined := strings.Join(joinedHistory, "\n")
	for _, want := range []string{"noninteractive run prompt explain the repo", "Read-only run flagged", "check git status --short --branch completed"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("history missing %q in %q", want, joined)
		}
	}
	if strings.Contains(joined, workspace) {
		t.Fatalf("history leaked absolute workspace path %q in %q", workspace, joined)
	}
}

func TestNonInteractiveWriteRunUsesAutonomyAndStoresMutationMemory(t *testing.T) {
	t.Parallel()

	workspace := seedNonInteractiveRunWorkspace(t)
	output, err := NonInteractiveRunCommandOutput(context.Background(), NonInteractiveRunRequest{
		Version:       "test-version",
		Prompt:        "create a note about the repo",
		WorkspacePath: workspace,
		AutonomyLevel: "write",
	})
	if err != nil {
		t.Fatalf("NonInteractiveRunCommandOutput returned error: %v", err)
	}
	for _, want := range []string{
		"mode: non_interactive_write",
		"status: flagged",
		"changed_files:",
		"- docs/aila-run-output.md status=completed bytes=",
		"mutation:",
		"- tool=write status=completed path=docs/aila-run-output.md bytes=",
		"decision_source=autonomy_policy autonomy=write allowed=true automatic=true approval_required=false",
		"stored_session: true",
		"stored_history: true",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("write output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, workspace) {
		t.Fatalf("write output leaked absolute workspace path %q:\n%s", workspace, output)
	}
	writtenPath := filepath.Join(workspace, "docs", "aila-run-output.md")
	content, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if got := string(content); !strings.Contains(got, "Prompt: create a note about the repo") || !strings.Contains(got, "bounded deterministic non-interactive write run") {
		t.Fatalf("written content = %q", got)
	}

	store := mustOpenProjectStoreForRun(t, workspace)
	snapshot, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("ReadCurrentSessionSnapshot returned error: %v", err)
	}
	run := snapshot.Snapshot.Run
	if snapshot.State != state.SessionSnapshotLoaded || run == nil || run.Mode != "non_interactive_write" {
		t.Fatalf("snapshot state=%q run=%#v, want write run memory", snapshot.State, run)
	}
	if len(run.ChangedFiles) != 1 || run.ChangedFiles[0].Path != "docs/aila-run-output.md" || run.Mutation == nil {
		t.Fatalf("snapshot changed/mutation evidence = changed=%#v mutation=%#v", run.ChangedFiles, run.Mutation)
	}
	if run.Mutation.Status != "completed" || run.Mutation.DecisionSource != "autonomy_policy" || run.Mutation.DecisionAutonomy != "write" || !run.Mutation.Allowed || run.Mutation.ApprovalRequired {
		t.Fatalf("snapshot mutation evidence = %#v", run.Mutation)
	}

	historyResult, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("ReadFakeHistory returned error: %v", err)
	}
	var mutationEvent *history.FakeEvent
	for index := range historyResult.Events {
		if historyResult.Events[index].Kind == history.EventKindMutation {
			mutationEvent = &historyResult.Events[index]
		}
	}
	if mutationEvent == nil || mutationEvent.Mutation == nil || mutationEvent.Undo == nil {
		t.Fatalf("history missing mutation/undo event: %#v", historyResult.Events)
	}
	if mutationEvent.Mutation.Status != "completed" || mutationEvent.Mutation.CommandSource != "noninteractive.run" || !mutationEvent.Undo.Available || mutationEvent.Undo.Action != "delete_created_file" {
		t.Fatalf("history mutation event = %#v undo=%#v", mutationEvent.Mutation, mutationEvent.Undo)
	}
}

func TestNonInteractiveWriteRunDeniedByReadAutonomyDoesNotMutate(t *testing.T) {
	t.Parallel()

	workspace := seedNonInteractiveRunWorkspace(t)
	output, err := NonInteractiveRunCommandOutput(context.Background(), NonInteractiveRunRequest{
		Version:       "test-version",
		Prompt:        "create a note about the repo",
		WorkspacePath: workspace,
		AutonomyLevel: "read",
	})
	if err != nil {
		t.Fatalf("NonInteractiveRunCommandOutput returned error: %v", err)
	}
	for _, want := range []string{
		"mode: non_interactive_write",
		"status: blocked",
		"changed_files:\n- none",
		"mutation:",
		"- tool=write status=denied path=docs/aila-run-output.md bytes=0",
		"decision_source=autonomy_policy autonomy=read allowed=false automatic=false approval_required=true",
		"blockers:",
		"write not completed: read autonomy requires approval for write-shaped operation",
		"stored_session: true",
		"stored_history: true",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("denied output missing %q:\n%s", want, output)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "docs", "aila-run-output.md")); !os.IsNotExist(err) {
		t.Fatalf("denied run should not create target, stat err=%v", err)
	}

	store := mustOpenProjectStoreForRun(t, workspace)
	snapshot, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("ReadCurrentSessionSnapshot returned error: %v", err)
	}
	run := snapshot.Snapshot.Run
	if snapshot.State != state.SessionSnapshotLoaded || run == nil || run.Mutation == nil {
		t.Fatalf("snapshot state=%q run=%#v, want denied write run memory", snapshot.State, run)
	}
	if run.Mutation.Status != "denied" || run.Mutation.DecisionAutonomy != "read" || run.Mutation.Allowed || !run.Mutation.ApprovalRequired || len(run.ChangedFiles) != 0 {
		t.Fatalf("denied snapshot mutation = %#v changed=%#v", run.Mutation, run.ChangedFiles)
	}
}

func TestNonInteractiveRunMissingPromptReturnsBoundedBlockedReport(t *testing.T) {
	t.Parallel()

	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	output, err := NonInteractiveRunCommandOutput(context.Background(), NonInteractiveRunRequest{
		Version:       "test-version",
		Prompt:        "  ",
		WorkspacePath: workspace,
	})
	if err != nil {
		t.Fatalf("NonInteractiveRunCommandOutput returned error: %v", err)
	}
	for _, want := range []string{
		"command: run",
		"mode: non_interactive_read_only",
		"status: blocked",
		"blockers:\n- prompt is required",
		"stored_session: false",
		"stored_history: false",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("blocked output missing %q:\n%s", want, output)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, ".aila")); !os.IsNotExist(err) {
		t.Fatalf("missing prompt should not create project store, stat err=%v", err)
	}
}

func seedNonInteractiveRunWorkspace(t *testing.T) string {
	t.Helper()
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	files := map[string]string{
		"README.md":  "# Test Repo\n\nA small repo for read-only run inspection.\n",
		"ROADMAP.md": "# Roadmap\n\n- current milestone: non-interactive read-only run\n",
		"AGENTS.md":  "# Agent Instructions\n\nKeep tests deterministic.\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	runGitForRunTest(t, workspace, "init")
	return workspace
}

func runGitForRunTest(t *testing.T, workspace string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func mustOpenProjectStoreForRun(t *testing.T, workspace string) state.Store {
	t.Helper()
	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open project store: %v", err)
	}
	return store
}

func containsText(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

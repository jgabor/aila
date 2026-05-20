package app

import (
	"context"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

func TestShellPrefixPromptRoutesThroughBashPermissionHistoryAndSnapshot(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeAppTestFile(t, workspace, "notes.txt", "alpha\n")
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyRead))
	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), snapshotTestView(), runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	})

	turn := controller.submitPrompt("!ls -1")

	if turn.UserText != "!ls -1" || turn.Command == nil || turn.Command.Status != "completed" || !turn.Command.ReadOnly {
		t.Fatalf("shell prefix turn = %+v", turn)
	}
	if !strings.Contains(turn.AssistantText, "notes.txt") {
		t.Fatalf("assistant text = %q, want notes.txt", turn.AssistantText)
	}
	if strings.Join(turn.Command.Argv, " ") != "ls -1" || !containsAnyString(turn.Command.StdoutLines, "notes.txt") {
		t.Fatalf("command view = %+v, want ls output", turn.Command)
	}
	if got := runner.model.LastBash; got.ToolName != "bash" || got.CommandFamily != "ls" || got.Source.Caller != shellPrefixSource {
		t.Fatalf("last bash = %+v, want shell-prefix bash result", got)
	}
	if len(historyEvents) != 1 || historyEvents[0].Event.Kind != history.EventKindCommand || historyEvents[0].Event.Source != shellPrefixSource || !strings.Contains(historyEvents[0].Event.DisplayText, "shell-prefix shell completed !ls -1") {
		t.Fatalf("history events = %+v", historyEvents)
	}
	if len(snapshots) == 0 || len(snapshots[len(snapshots)-1].Snapshot.Transcript) < 2 {
		t.Fatalf("snapshots = %+v, want persisted shell prefix transcript", snapshots)
	}
	lastTranscript := snapshots[len(snapshots)-1].Snapshot.Transcript
	if lastTranscript[0].Role != "user" || lastTranscript[0].Text != "!ls -1" || lastTranscript[1].Role != "assistant" {
		t.Fatalf("snapshot transcript = %+v", lastTranscript)
	}
}

func TestShellPrefixPromptSurfacesValidationFailureThroughBashResult(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithReadContext(t.Context(), t.TempDir(), string(permission.AutonomyRead))
	controller := newSessionControllerWithPersistence(context.Background(), snapshotTestView(), runner, nil)

	turn := controller.submitPrompt("!git checkout main")

	if turn.Command == nil || turn.Command.Status != "failed" || turn.Command.ErrorKind != string(runtime.BashToolErrorUnsafeCommand) {
		t.Fatalf("shell prefix failure turn = %+v", turn)
	}
	if got := runner.model.LastBash.Error; got.Kind != runtime.BashToolErrorUnsafeCommand || !strings.Contains(got.Message, "git subcommand") {
		t.Fatalf("last bash error = %+v", got)
	}
}

func TestSummarizedShellPrefixRoutesThroughBashPermissionHistoryAndContext(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeAppTestFile(t, workspace, "notes.txt", "alpha\n")
	runner := newInputRunnerWithReadContext(t.Context(), workspace, string(permission.AutonomyRead))
	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	controller := newSessionControllerWithPersistenceAndHistory(context.Background(), snapshotTestView(), runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	})

	turn := controller.submitPrompt("!!ls -1")

	if turn.UserText != "!!ls -1" || turn.Command == nil || turn.Command.Status != "completed" || turn.Command.CommandFamily != "summarized shell" {
		t.Fatalf("summarized shell turn = %+v", turn)
	}
	if !strings.Contains(turn.AssistantText, "notes.txt") {
		t.Fatalf("assistant text = %q, want notes.txt", turn.AssistantText)
	}
	if runner.model.LastCompact.Summary == "" || !strings.Contains(runner.model.LastCompact.Summary, "ls -1 completed exit 0") {
		t.Fatalf("model.LastCompact.Summary = %q", runner.model.LastCompact.Summary)
	}
	hasTranscriptContextBlock := false
	for _, entry := range runner.model.Transcript {
		if entry.Kind == "prompt" && strings.Contains(entry.Text, "Context Block") && strings.Contains(entry.Text, "notes.txt") {
			hasTranscriptContextBlock = true
			break
		}
	}
	if !hasTranscriptContextBlock {
		t.Fatalf("transcript entries = %+v, want prompt entry with notes.txt context block", runner.model.Transcript)
	}
	if runner.model.LastBash.ToolName != "bash" || runner.model.LastBash.Source.Caller != shellPrefixSource || !containsAnyString(turn.Command.StdoutLines, "notes.txt") {
		t.Fatalf("last bash=%+v command=%+v, want summarized shell output", runner.model.LastBash, turn.Command)
	}
	if turn.Context == nil || turn.Context.Source != "app.context" || !strings.Contains(turn.Context.Meter, "refs") || len(turn.Context.Claims) == 0 {
		t.Fatalf("context view = %+v, want source-backed summarized shell context", turn.Context)
	}
	if !contextHasSourceRef(turn.Context, "command-1-stdout-1", "notes.txt") || !contextHasClaimRef(turn.Context, "command ls -1 completed exit 0", "command-1-stdout-1") {
		t.Fatalf("context refs/claims = %+v", turn.Context)
	}
	if len(historyEvents) != 1 || historyEvents[0].Event.Kind != history.EventKindCommand || !strings.Contains(historyEvents[0].Event.DisplayText, "shell-prefix summarized_shell completed !!ls -1") {
		t.Fatalf("history events = %+v", historyEvents)
	}
	if len(snapshots) == 0 || len(snapshots[len(snapshots)-1].Snapshot.Transcript) < 2 {
		t.Fatalf("snapshots = %+v, want persisted summarized shell transcript", snapshots)
	}
}

func TestSummarizedShellPrefixPreservesFailureContext(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithReadContext(t.Context(), t.TempDir(), string(permission.AutonomyRead))
	controller := newSessionControllerWithPersistence(context.Background(), snapshotTestView(), runner, nil)

	turn := controller.submitPrompt("!!git checkout main")

	if turn.Command == nil || turn.Command.Status != "failed" || turn.Command.ErrorKind != string(runtime.BashToolErrorUnsafeCommand) {
		t.Fatalf("summarized shell failure turn = %+v", turn)
	}
	if turn.Context == nil || !contextHasSourceRef(turn.Context, "command-1-failure", "git subcommand") || !contextHasClaimRef(turn.Context, "command git checkout main failed: unsafe_command: git subcommand is not allowed", "command-1-failure") {
		t.Fatalf("failure context = %+v", turn.Context)
	}
}

func TestOrdinaryPromptBypassesShellPrefixRouting(t *testing.T) {
	t.Parallel()

	runner := newInputRunnerWithDispatch(runtime.Dispatch)
	controller := newSessionControllerWithPersistence(context.Background(), snapshotTestView(), runner, nil)

	turn := controller.submitPrompt("explain status")

	if turn.UserText != "explain status" || turn.Command != nil || runner.model.LastBash.ToolName != "" {
		t.Fatalf("ordinary prompt turn=%+v model=%+v", turn, runner.model)
	}
}

func contextHasSourceRef(contextView *tui.ContextView, id string, excerpt string) bool {
	if contextView == nil {
		return false
	}
	for _, ref := range contextView.SourceRefs {
		if ref.ID == id && strings.Contains(ref.Excerpt, excerpt) {
			return true
		}
	}
	return false
}

func contextHasClaimRef(contextView *tui.ContextView, claimText string, refID string) bool {
	if contextView == nil {
		return false
	}
	for _, claim := range contextView.Claims {
		if claim.Text != claimText {
			continue
		}
		for _, id := range claim.SourceRefIDs {
			if id == refID {
				return true
			}
		}
	}
	return false
}

func TestShellPrefixArgvParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    `git commit -m "hello world"`,
			expected: []string{"git", "commit", "-m", "hello world"},
		},
		{
			input:    `git commit -m 'single quoted'`,
			expected: []string{"git", "commit", "-m", "single quoted"},
		},
		{
			input:    `echo "escaped \" quotes"`,
			expected: []string{"echo", `escaped " quotes`},
		},
		{
			input:    `echo 'no escaping inside single quotes \'`,
			expected: []string{"echo", `no escaping inside single quotes \`},
		},
		{
			input:    `echo  multiple   spaces  `,
			expected: []string{"echo", "multiple", "spaces"},
		},
	}

	for _, tc := range tests {
		got := shellPrefixArgv(tc.input)
		if len(got) != len(tc.expected) {
			t.Errorf("shellPrefixArgv(%q) length = %d, want %d", tc.input, len(got), len(tc.expected))
			continue
		}
		for i := range got {
			if got[i] != tc.expected[i] {
				t.Errorf("shellPrefixArgv(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.expected[i])
			}
		}
	}
}

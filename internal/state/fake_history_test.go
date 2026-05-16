package state

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/history"
)

func TestReadFakeHistoryReportsEmptyWhenMissing(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	result, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("ReadFakeHistory returned error: %v", err)
	}
	if result.State != FakeHistoryEmpty {
		t.Fatalf("read state = %q, want %q", result.State, FakeHistoryEmpty)
	}
	if len(result.Events) != 0 || len(result.Diagnostics) != 0 {
		t.Fatalf("missing history result = %#v, want empty without diagnostics", result)
	}
	assertStoreEntries(t, store.Layout().StoreRoot, []string{"artifacts/", "indexes/", "project.toml"})
}

func TestAppendAndReadFakeHistoryRoundTripsStableBoundedRedactedEvents(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	first := validFakeHistoryEvent("event-1", history.EventKindPrompt)
	first.DisplayText = "hello token=abc123"
	second := validFakeHistoryEvent("event-2", history.EventKindResponse)
	second.DisplayText = "answer Authorization: Bearer sk-live"

	for _, event := range []history.FakeEvent{first, second} {
		result, err := store.AppendFakeHistory(context.Background(), event)
		if err != nil {
			t.Fatalf("AppendFakeHistory returned error: %v", err)
		}
		if result.State != FakeHistoryLoaded || len(result.Diagnostics) != 0 {
			t.Fatalf("append result = %#v, want loaded without diagnostics", result)
		}
		if result.Location.Provenance.RelativePath != "history/fake-events.jsonl" {
			t.Fatalf("history provenance = %#v", result.Location.Provenance)
		}
	}

	location := mustCurrentFakeHistoryLocation(t, store.Layout())
	assertFileExists(t, location.Path)
	assertNoFakeHistoryTempFiles(t, filepath.Dir(location.Path))
	assertStoreEntries(t, store.Layout().StoreRoot, []string{"artifacts/", "history/", "indexes/", "project.toml"})

	content, err := os.ReadFile(location.Path)
	if err != nil {
		t.Fatalf("read fake history file: %v", err)
	}
	if !strings.HasSuffix(string(content), "\n") || strings.Count(string(content), "\n") != 2 {
		t.Fatalf("history JSONL content = %q, want two newline-terminated records", content)
	}
	for _, leaked := range []string{"abc123", "sk-live", "token=", "Authorization:"} {
		if strings.Contains(string(content), leaked) {
			t.Fatalf("history file leaked %q in %q", leaked, content)
		}
	}

	read, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("ReadFakeHistory returned error: %v", err)
	}
	if read.State != FakeHistoryLoaded || len(read.Diagnostics) != 0 {
		t.Fatalf("read result = %#v, want loaded without diagnostics", read)
	}
	gotIDs := []string{read.Events[0].EventID, read.Events[1].EventID}
	if !reflect.DeepEqual(gotIDs, []string{"event-1", "event-2"}) {
		t.Fatalf("event order = %v", gotIDs)
	}
	if read.Events[0].DisplayText != "hello [secret]" || read.Events[1].DisplayText != "answer [secret]" {
		t.Fatalf("redacted display text = %#v", read.Events)
	}
}

func TestAppendAndReadFakeHistoryRoundTripsMutationUndoMetadata(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	event := validFakeHistoryEvent("event-mutation", history.EventKindMutation)
	event.Source = "mutation.tool"
	event.Provenance = "mutation.result"
	event.DisplayText = "mutation write completed notes.txt token=abc123"
	event.Mutation = &history.MutationRecord{
		ToolName:              "write",
		Status:                "completed",
		CommandSource:         "fake-approval",
		RequestID:             "write-1",
		ApprovalID:            "approval-1",
		ApprovalAction:        "approve",
		ChangedPaths:          []string{"notes.txt"},
		RequestedPath:         "notes.txt",
		ExpectedEffect:        "create notes Authorization: Bearer sk-live",
		PreviousVersion:       "missing",
		NewVersion:            "sha256:abc",
		BytesWritten:          6,
		ResolvedPathAvailable: true,
		DecisionRunID:         "op-1",
		DecisionCapability:    "fake-approval",
	}
	event.Undo = &history.UndoMetadata{
		Available:       true,
		Action:          "delete_created_file",
		Paths:           []string{"notes.txt"},
		PreviousVersion: "missing",
		NewVersion:      "sha256:abc",
	}

	if _, err := store.AppendFakeHistory(context.Background(), event); err != nil {
		t.Fatalf("AppendFakeHistory returned error: %v", err)
	}
	read, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("ReadFakeHistory returned error: %v", err)
	}
	if read.State != FakeHistoryLoaded || len(read.Events) != 1 {
		t.Fatalf("read mutation history = %#v, want one loaded event", read)
	}
	got := read.Events[0]
	if got.Mutation == nil || got.Undo == nil {
		t.Fatalf("mutation event missing structured metadata: %#v", got)
	}
	if got.Mutation.ApprovalID != "approval-1" || !reflect.DeepEqual(got.Mutation.ChangedPaths, []string{"notes.txt"}) || !got.Undo.Available || got.Undo.Action != "delete_created_file" {
		t.Fatalf("mutation/undo metadata = mutation %#v undo %#v", got.Mutation, got.Undo)
	}
	content, err := os.ReadFile(mustCurrentFakeHistoryLocation(t, store.Layout()).Path)
	if err != nil {
		t.Fatalf("read fake history JSONL: %v", err)
	}
	for _, marker := range []string{"\"kind\":\"mutation\"", "\"approval_id\":\"approval-1\"", "\"changed_paths\":[\"notes.txt\"]", "\"available\":true"} {
		if !strings.Contains(string(content), marker) {
			t.Fatalf("history JSONL missing %q: %s", marker, content)
		}
	}
	for _, leaked := range []string{"token=", "abc123", "Authorization:", "sk-live"} {
		if strings.Contains(string(content), leaked) {
			t.Fatalf("history JSONL leaked %q: %s", leaked, content)
		}
	}
}

func TestAppendAndReadFakeHistoryRoundTripsRecoveryMetadata(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	event := validFakeHistoryEvent("event-recovery", history.EventKindRecovery)
	event.Source = "recovery.command"
	event.Provenance = "recovery.undo"
	event.DisplayText = "recovery undo completed notes.txt token=abc123"
	event.Recovery = &history.RecoveryRecord{
		Command:            "undo",
		Status:             "completed",
		TargetEventID:      "event-mutation",
		Action:             "delete_created_file",
		Paths:              []string{"notes.txt"},
		PreviousVersion:    "sha256:abc",
		NewVersion:         "missing",
		RedoAvailable:      true,
		RedoAction:         "restore_created_file",
		RedoContent:        "restored notes Authorization: Bearer sk-live",
		DecisionRunID:      "op-undo-1",
		DecisionCapability: "recovery.undo",
	}

	if _, err := store.AppendFakeHistory(context.Background(), event); err != nil {
		t.Fatalf("AppendFakeHistory returned error: %v", err)
	}
	read, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("ReadFakeHistory returned error: %v", err)
	}
	if read.State != FakeHistoryLoaded || len(read.Events) != 1 {
		t.Fatalf("read recovery history = %#v, want one loaded event", read)
	}
	got := read.Events[0]
	if got.Recovery == nil {
		t.Fatalf("recovery event missing structured metadata: %#v", got)
	}
	if got.Recovery.Command != "undo" || got.Recovery.Status != "completed" || got.Recovery.TargetEventID != "event-mutation" {
		t.Fatalf("recovery command/status/target = %#v", got.Recovery)
	}
	if got.Recovery.Action != "delete_created_file" || !reflect.DeepEqual(got.Recovery.Paths, []string{"notes.txt"}) || !got.Recovery.RedoAvailable || got.Recovery.RedoAction != "restore_created_file" {
		t.Fatalf("recovery action/redo metadata = %#v", got.Recovery)
	}
	content, err := os.ReadFile(mustCurrentFakeHistoryLocation(t, store.Layout()).Path)
	if err != nil {
		t.Fatalf("read fake history JSONL: %v", err)
	}
	for _, marker := range []string{"\"kind\":\"recovery\"", "\"command\":\"undo\"", "\"target_event_id\":\"event-mutation\"", "\"redo_available\":true"} {
		if !strings.Contains(string(content), marker) {
			t.Fatalf("history JSONL missing %q: %s", marker, content)
		}
	}
	for _, leaked := range []string{"token=", "abc123", "Authorization:", "sk-live"} {
		if strings.Contains(string(content), leaked) {
			t.Fatalf("history JSONL leaked %q: %s", leaked, content)
		}
	}
}

func TestReadAndAppendFakeHistoryReturnRecoveryWithoutOverwrite(t *testing.T) {
	t.Parallel()

	validLine := strings.TrimSuffix(string(mustMarshalFakeHistoryEvent(t, validFakeHistoryEvent("event-1", history.EventKindPrompt))), "\n")
	versionMismatch := validFakeHistoryEvent("event-1", history.EventKindPrompt)
	versionMismatch.SchemaVersion = history.FakeEventSchemaVersion + 1
	tests := map[string]string{
		"corrupt JSONL":     "{not-json}\n",
		"partial event":     validLine,
		"version mismatch":  string(mustMarshalFakeHistoryEvent(t, versionMismatch)),
		"oversized history": strings.Repeat("x", FakeHistoryMaxFileBytes+1),
	}
	for name, content := range tests {
		name, content := name, content
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
			location := mustCurrentFakeHistoryLocation(t, store.Layout())
			seedFakeHistory(t, location.Path, content)
			unrelatedPath := filepath.Join(store.Layout().ArtifactsRoot, "keep.txt")
			if err := os.WriteFile(unrelatedPath, []byte("keep"), 0o644); err != nil {
				t.Fatalf("seed unrelated store data: %v", err)
			}

			read, err := store.ReadFakeHistory(context.Background())
			if err != nil {
				t.Fatalf("ReadFakeHistory returned error: %v", err)
			}
			assertFakeHistoryRecovery(t, read.State, read.Diagnostics)

			appendResult, err := store.AppendFakeHistory(context.Background(), validFakeHistoryEvent("event-2", history.EventKindRuntime))
			if err != nil {
				t.Fatalf("AppendFakeHistory returned error: %v", err)
			}
			assertFakeHistoryRecovery(t, appendResult.State, appendResult.Diagnostics)
			assertFileContent(t, location.Path, content)
			assertFileContent(t, unrelatedPath, "keep")
			assertNoFakeHistoryTempFiles(t, filepath.Dir(location.Path))
		})
	}
}

func TestFakeHistoryReadAndAppendRejectSymlinkEscapeWithoutExternalMutation(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	location := mustCurrentFakeHistoryLocation(t, store.Layout())
	externalDir := filepath.Join(t.TempDir(), "external")
	if err := os.MkdirAll(externalDir, 0o755); err != nil {
		t.Fatalf("create external dir: %v", err)
	}
	if err := os.Symlink(externalDir, filepath.Dir(location.Path)); err != nil {
		t.Fatalf("create history symlink: %v", err)
	}
	externalPath := filepath.Join(externalDir, "fake-events.jsonl")
	seedFakeHistory(t, externalPath, string(mustMarshalFakeHistoryEvent(t, validFakeHistoryEvent("event-1", history.EventKindPrompt))))

	read, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("ReadFakeHistory returned error: %v", err)
	}
	assertFakeHistoryRecovery(t, read.State, read.Diagnostics)

	appendResult, err := store.AppendFakeHistory(context.Background(), validFakeHistoryEvent("event-2", history.EventKindCommand))
	if err != nil {
		t.Fatalf("AppendFakeHistory returned error: %v", err)
	}
	assertFakeHistoryRecovery(t, appendResult.State, appendResult.Diagnostics)
	assertFileContent(t, externalPath, string(mustMarshalFakeHistoryEvent(t, validFakeHistoryEvent("event-1", history.EventKindPrompt))))
	assertNoFakeHistoryTempFiles(t, externalDir)
}

func TestAppendFakeHistoryRejectsValidLogThatWouldExceedBound(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	location := mustCurrentFakeHistoryLocation(t, store.Layout())
	line := string(mustMarshalFakeHistoryEvent(t, validFakeHistoryEvent("event-1", history.EventKindPrompt)))
	existing := strings.Repeat(line, FakeHistoryMaxFileBytes/len(line))
	seedFakeHistory(t, location.Path, existing)

	result, err := store.AppendFakeHistory(context.Background(), validFakeHistoryEvent("event-2", history.EventKindRuntime))
	if err != nil {
		t.Fatalf("AppendFakeHistory returned error: %v", err)
	}
	assertFakeHistoryRecovery(t, result.State, result.Diagnostics)
	assertFileContent(t, location.Path, existing)
	assertNoFakeHistoryTempFiles(t, filepath.Dir(location.Path))
}

func TestAppendFakeHistoryWriteFailureLeavesFinalHistoryUnchanged(t *testing.T) {
	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	location := mustCurrentFakeHistoryLocation(t, store.Layout())
	existing := string(mustMarshalFakeHistoryEvent(t, validFakeHistoryEvent("event-1", history.EventKindPrompt)))
	seedFakeHistory(t, location.Path, existing)

	originalWriter := writeFakeHistoryFileAtomic
	t.Cleanup(func() { writeFakeHistoryFileAtomic = originalWriter })
	writeFakeHistoryFileAtomic = func(ctx context.Context, path string, content []byte) error {
		if path != location.Path {
			t.Fatalf("atomic write path = %q, want %q", path, location.Path)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if got := string(content); !strings.HasPrefix(got, existing) || strings.Count(got, "\n") != 2 {
			t.Fatalf("rebuilt history content = %q, want existing plus one complete line", got)
		}
		return errors.New("injected atomic write failure")
	}

	_, err := store.AppendFakeHistory(context.Background(), validFakeHistoryEvent("event-2", history.EventKindRuntime))
	if err == nil || !strings.Contains(err.Error(), "injected atomic write failure") {
		t.Fatalf("AppendFakeHistory error = %v, want injected write failure", err)
	}
	assertFileContent(t, location.Path, existing)
	assertNoFakeHistoryTempFiles(t, filepath.Dir(location.Path))
}

func TestAppendFakeHistoryRejectsInvalidEventWithoutMutationOrTempFile(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	location := mustCurrentFakeHistoryLocation(t, store.Layout())
	const existing = "{not-json}\n"
	seedFakeHistory(t, location.Path, existing)

	invalid := validFakeHistoryEvent("", history.EventKindPrompt)
	if _, err := store.AppendFakeHistory(context.Background(), invalid); !errors.Is(err, history.ErrInvalidFakeEvent) {
		t.Fatalf("invalid append error = %v, want ErrInvalidFakeEvent", err)
	}
	assertFileContent(t, location.Path, existing)
	assertNoFakeHistoryTempFiles(t, filepath.Dir(location.Path))
}

func validFakeHistoryEvent(eventID string, kind history.EventKind) history.FakeEvent {
	return history.FakeEvent{
		SchemaVersion: history.FakeEventSchemaVersion,
		Kind:          kind,
		EventID:       eventID,
		RunID:         "run-1",
		SessionID:     "session-1",
		Source:        "fake-runtime",
		Provenance:    "state-test",
		DisplayText:   "visible fake activity",
	}
}

func mustCurrentFakeHistoryLocation(t *testing.T, layout Layout) FakeHistoryLocation {
	t.Helper()
	location, err := CurrentFakeHistoryLocation(layout)
	if err != nil {
		t.Fatalf("CurrentFakeHistoryLocation returned error: %v", err)
	}
	return location
}

func seedFakeHistory(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fake history directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seed fake history: %v", err)
	}
}

func mustMarshalFakeHistoryEvent(t *testing.T, event history.FakeEvent) []byte {
	t.Helper()
	content, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal fake history event: %v", err)
	}
	return append(content, '\n')
}

func assertFakeHistoryRecovery(t *testing.T, state FakeHistoryReadState, diagnostics []diagnostic.Diagnostic) {
	t.Helper()
	if state != FakeHistoryRecoveryNeeded {
		t.Fatalf("state = %q, want %q", state, FakeHistoryRecoveryNeeded)
	}
	if len(diagnostics) != 1 || !diagnostics[0].UserInputNeeded {
		t.Fatalf("diagnostics = %#v, want one actionable recovery diagnostic", diagnostics)
	}
	got := diagnostics[0]
	if got.Category != diagnostic.CategoryState || got.Source != diagnostic.SourceStateHistory || got.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic identity = %#v", got)
	}
	if got.AffectedArtifact != diagnostic.ArtifactFakeHistory || got.RecoveryAction != diagnostic.RecoveryManualRepair {
		t.Fatalf("diagnostic recovery fields = %#v", got)
	}
}

func assertNoFakeHistoryTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read fake history dir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".fake-events.jsonl.") || strings.HasSuffix(name, ".tmp") {
			t.Fatalf("leftover fake history temp file %s", filepath.Join(dir, name))
		}
	}
}

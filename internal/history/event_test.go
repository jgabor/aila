package history

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestEventKindsAreClosedAndStable(t *testing.T) {
	t.Parallel()

	want := []EventKind{EventKindPrompt, EventKindResponse, EventKindCommand, EventKindRuntime, EventKindMutation}
	if got := EventKinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("EventKinds() = %v, want %v", got, want)
	}
	if got := []string{string(EventKindPrompt), string(EventKindResponse), string(EventKindCommand), string(EventKindRuntime), string(EventKindMutation)}; !reflect.DeepEqual(got, []string{"prompt", "response", "command", "runtime", "mutation"}) {
		t.Fatalf("stable event kind IDs = %v", got)
	}
}

func TestNormalizeFakeEventRequiresStableIdentifiersAndProvenance(t *testing.T) {
	t.Parallel()

	event, err := NormalizeFakeEvent(validFakeEvent(EventKindPrompt))
	if err != nil {
		t.Fatalf("NormalizeFakeEvent returned error: %v", err)
	}
	if event.SchemaVersion != FakeEventSchemaVersion || event.EventID == "" || event.RunID == "" || event.SessionID == "" || event.Source == "" || event.Provenance == "" {
		t.Fatalf("normalized event missing stable contract fields: %#v", event)
	}

	for name, mutate := range map[string]func(*FakeEvent){
		"schema_version": func(event *FakeEvent) { event.SchemaVersion = FakeEventSchemaVersion + 1 },
		"kind":           func(event *FakeEvent) { event.Kind = "model" },
		"event_id":       func(event *FakeEvent) { event.EventID = "" },
		"run_id":         func(event *FakeEvent) { event.RunID = strings.Repeat("r", RunIDMaxBytes+1) },
		"session_id":     func(event *FakeEvent) { event.SessionID = " " },
		"source":         func(event *FakeEvent) { event.Source = strings.Repeat("s", SourceMaxBytes+1) },
		"provenance":     func(event *FakeEvent) { event.Provenance = strings.Repeat("p", ProvenanceMaxBytes+1) },
		"display_text":   func(event *FakeEvent) { event.DisplayText = "\x1b[31m\x07" },
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			event := validFakeEvent(EventKindResponse)
			mutate(&event)
			if _, err := NormalizeFakeEvent(event); !errors.Is(err, ErrInvalidFakeEvent) {
				t.Fatalf("%s error = %v, want ErrInvalidFakeEvent", name, err)
			}
		})
	}
}

func TestNormalizeFakeEventBoundsRedactsAndStripsControls(t *testing.T) {
	t.Parallel()

	event := validFakeEvent(EventKindCommand)
	event.DisplayText = "\x1b[31mrun\x1b[0m\n\x1b]0;hidden title\x07\u009b31mred Authorization: Bearer sk-live password=hunter2 token=abc123 to\u009bken=split123\x07 " + strings.Repeat("x", DisplayTextMaxBytes)
	normalized, err := NormalizeFakeEvent(event)
	if err != nil {
		t.Fatalf("NormalizeFakeEvent returned error: %v", err)
	}
	if len(normalized.DisplayText) > DisplayTextMaxBytes {
		t.Fatalf("display_text length = %d, want <= %d", len(normalized.DisplayText), DisplayTextMaxBytes)
	}
	for _, leaked := range []string{"\x1b", "\x07", "\u009b", "hidden title", "sk-live", "hunter2", "abc123", "split123", "Authorization:", "password=", "token="} {
		if strings.Contains(normalized.DisplayText, leaked) {
			t.Fatalf("display_text leaked %q in %q", leaked, normalized.DisplayText)
		}
	}
	if !strings.Contains(normalized.DisplayText, "run") || !strings.Contains(normalized.DisplayText, "red") || !strings.Contains(normalized.DisplayText, "[secret]") {
		t.Fatalf("display_text did not preserve bounded redacted text: %q", normalized.DisplayText)
	}
}

func TestNormalizeFakeMutationEventRecordsUndoMetadata(t *testing.T) {
	t.Parallel()

	event := validMutationFakeEvent()
	event.Mutation.ExpectedEffect = "create notes token=abc123"
	event.Mutation.ErrorMessage = "Authorization: Bearer sk-live"
	normalized, err := NormalizeFakeEvent(event)
	if err != nil {
		t.Fatalf("NormalizeFakeEvent mutation returned error: %v", err)
	}
	if normalized.Mutation == nil || normalized.Undo == nil {
		t.Fatalf("normalized mutation metadata missing: %#v", normalized)
	}
	if normalized.Mutation.ToolName != "write" || normalized.Mutation.Status != "completed" || !reflect.DeepEqual(normalized.Mutation.ChangedPaths, []string{"notes.txt"}) {
		t.Fatalf("normalized mutation = %#v", normalized.Mutation)
	}
	if normalized.Mutation.ApprovalID != "approval-1" || normalized.Mutation.ApprovalAction != "approve" || normalized.Mutation.CommandSource != "fake-approval" {
		t.Fatalf("approval/source metadata = %#v", normalized.Mutation)
	}
	if !normalized.Undo.Available || normalized.Undo.Action != "delete_created_file" || !reflect.DeepEqual(normalized.Undo.Paths, []string{"notes.txt"}) {
		t.Fatalf("undo metadata = %#v", normalized.Undo)
	}
	for _, leaked := range []string{"token=", "abc123", "Authorization:", "sk-live"} {
		if strings.Contains(normalized.Mutation.ExpectedEffect, leaked) || strings.Contains(normalized.Mutation.ErrorMessage, leaked) {
			t.Fatalf("mutation metadata leaked %q: %#v", leaked, normalized.Mutation)
		}
	}
}

func TestNormalizeFakeMutationEventRejectsUnsafeOrIncompleteMetadata(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*FakeEvent){
		"missing mutation": func(event *FakeEvent) { event.Mutation = nil },
		"missing undo":     func(event *FakeEvent) { event.Undo = nil },
		"no changed paths": func(event *FakeEvent) { event.Mutation.ChangedPaths = nil },
		"unsafe path":      func(event *FakeEvent) { event.Mutation.ChangedPaths = []string{"/home/user/notes.txt"} },
		"missing action":   func(event *FakeEvent) { event.Undo.Action = "" },
		"missing reason": func(event *FakeEvent) {
			event.Undo.Available = false
			event.Undo.Action = ""
			event.Undo.Paths = nil
			event.Undo.Reason = ""
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			event := validMutationFakeEvent()
			mutate(&event)
			if _, err := NormalizeFakeEvent(event); !errors.Is(err, ErrInvalidFakeEvent) {
				t.Fatalf("%s error = %v, want ErrInvalidFakeEvent", name, err)
			}
		})
	}
}

func TestNormalizeFakeEventRejectsMutationMetadataOnOtherKinds(t *testing.T) {
	t.Parallel()

	event := validFakeEvent(EventKindRuntime)
	event.Mutation = validMutationRecord()
	event.Undo = validUndoMetadata(true)
	if _, err := NormalizeFakeEvent(event); !errors.Is(err, ErrInvalidFakeEvent) {
		t.Fatalf("non-mutation metadata error = %v, want ErrInvalidFakeEvent", err)
	}
}

func TestFakeEventContractFieldsRemainNarrow(t *testing.T) {
	t.Parallel()

	want := []string{"SchemaVersion", "Kind", "EventID", "RunID", "SessionID", "Source", "Provenance", "DisplayText", "Mutation", "Undo"}
	eventType := reflect.TypeOf(FakeEvent{})
	got := make([]string, 0, eventType.NumField())
	for index := 0; index < eventType.NumField(); index++ {
		got = append(got, eventType.Field(index).Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FakeEvent fields = %v, want %v", got, want)
	}
}

func validFakeEvent(kind EventKind) FakeEvent {
	return FakeEvent{
		SchemaVersion: FakeEventSchemaVersion,
		Kind:          kind,
		EventID:       "event-1",
		RunID:         "run-1",
		SessionID:     "session-1",
		Source:        "fake-runtime",
		Provenance:    "app-command",
		DisplayText:   "visible fake activity",
	}
}

func validMutationFakeEvent() FakeEvent {
	event := validFakeEvent(EventKindMutation)
	event.Source = "mutation.tool"
	event.Provenance = "mutation.result"
	event.DisplayText = "mutation write completed notes.txt"
	event.Mutation = validMutationRecord()
	event.Undo = validUndoMetadata(true)
	return event
}

func validMutationRecord() *MutationRecord {
	return &MutationRecord{
		ToolName:              "write",
		Status:                "completed",
		CommandSource:         "fake-approval",
		RequestID:             "write-1",
		ApprovalID:            "approval-1",
		ApprovalAction:        "approve",
		ChangedPaths:          []string{"notes.txt"},
		RequestedPath:         "notes.txt",
		ExpectedEffect:        "create notes",
		PreviousVersion:       "missing",
		NewVersion:            "sha256:abc",
		PreviousExists:        false,
		BytesWritten:          6,
		ResolvedPathAvailable: true,
		DecisionRunID:         "op-1",
		DecisionCapability:    "fake-approval",
	}
}

func validUndoMetadata(available bool) *UndoMetadata {
	if !available {
		return &UndoMetadata{Available: false, Action: "restore_previous_content", Paths: []string{"notes.txt"}, Reason: "previous content not recorded"}
	}
	return &UndoMetadata{Available: true, Action: "delete_created_file", Paths: []string{"notes.txt"}, PreviousVersion: "missing", NewVersion: "sha256:abc"}
}

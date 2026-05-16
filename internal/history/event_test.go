package history

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestEventKindsAreClosedAndStable(t *testing.T) {
	t.Parallel()

	want := []EventKind{EventKindPrompt, EventKindResponse, EventKindCommand, EventKindRuntime}
	if got := EventKinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("EventKinds() = %v, want %v", got, want)
	}
	if got := []string{string(EventKindPrompt), string(EventKindResponse), string(EventKindCommand), string(EventKindRuntime)}; !reflect.DeepEqual(got, []string{"prompt", "response", "command", "runtime"}) {
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

func TestFakeEventContractFieldsRemainNarrow(t *testing.T) {
	t.Parallel()

	want := []string{"SchemaVersion", "Kind", "EventID", "RunID", "SessionID", "Source", "Provenance", "DisplayText"}
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

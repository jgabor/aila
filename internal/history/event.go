package history

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	FakeEventSchemaVersion = 1
	EventIDMaxBytes        = 128
	RunIDMaxBytes          = 128
	SessionIDMaxBytes      = 128
	SourceMaxBytes         = 64
	ProvenanceMaxBytes     = 256
	DisplayTextMaxBytes    = 512
)

var ErrInvalidFakeEvent = errors.New("fake history event contract is invalid")

// EventKind is the closed set of fake activity event kinds supported by M17.
type EventKind string

const (
	EventKindPrompt   EventKind = "prompt"
	EventKindResponse EventKind = "response"
	EventKindCommand  EventKind = "command"
	EventKindRuntime  EventKind = "runtime"
)

// FakeEvent is a bounded, display-safe record of fake activity.
type FakeEvent struct {
	SchemaVersion int       `json:"schema_version"`
	Kind          EventKind `json:"kind"`
	EventID       string    `json:"event_id"`
	RunID         string    `json:"run_id"`
	SessionID     string    `json:"session_id"`
	Source        string    `json:"source"`
	Provenance    string    `json:"provenance"`
	DisplayText   string    `json:"display_text"`
}

// EventKinds returns the stable, closed event-kind set in display order.
func EventKinds() []EventKind {
	return []EventKind{EventKindPrompt, EventKindResponse, EventKindCommand, EventKindRuntime}
}

// NormalizeFakeEvent validates an event and returns bounded display-safe text.
func NormalizeFakeEvent(event FakeEvent) (FakeEvent, error) {
	if event.SchemaVersion != FakeEventSchemaVersion {
		return FakeEvent{}, fmt.Errorf("%w: unsupported schema_version %d", ErrInvalidFakeEvent, event.SchemaVersion)
	}
	if !validEventKind(event.Kind) {
		return FakeEvent{}, fmt.Errorf("%w: unknown kind %q", ErrInvalidFakeEvent, event.Kind)
	}
	for _, check := range []struct {
		field string
		value string
		limit int
	}{
		{field: "event_id", value: event.EventID, limit: EventIDMaxBytes},
		{field: "run_id", value: event.RunID, limit: RunIDMaxBytes},
		{field: "session_id", value: event.SessionID, limit: SessionIDMaxBytes},
		{field: "source", value: event.Source, limit: SourceMaxBytes},
		{field: "provenance", value: event.Provenance, limit: ProvenanceMaxBytes},
	} {
		if err := requiredBoundedString(check.field, check.value, check.limit); err != nil {
			return FakeEvent{}, err
		}
	}

	event.DisplayText = boundedDisplayText(redactSecrets(stripTerminalControls(event.DisplayText)))
	if strings.TrimSpace(event.DisplayText) == "" {
		return FakeEvent{}, fmt.Errorf("%w: display_text is empty", ErrInvalidFakeEvent)
	}
	return event, nil
}

func validEventKind(kind EventKind) bool {
	for _, candidate := range EventKinds() {
		if kind == candidate {
			return true
		}
	}
	return false
}

func requiredBoundedString(field string, value string, maxBytes int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s is empty", ErrInvalidFakeEvent, field)
	}
	if len(value) > maxBytes {
		return fmt.Errorf("%w: %s exceeds %d bytes", ErrInvalidFakeEvent, field, maxBytes)
	}
	return nil
}

var (
	terminalPattern      = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\)|[@-Z\\-_])`)
	authorizationPattern = regexp.MustCompile(`(?i)\bauthorization\b\s*(?::|=|\s+)\s*(?:bearer|basic)?\s*\S+`)
	secretPattern        = regexp.MustCompile(`(?i)\b(?:api[_-]?key|apikey|password|secret|token)\b\s*(?::|=|\s+)\s*\S+`)
)

func stripTerminalControls(value string) string {
	value = terminalPattern.ReplaceAllString(value, "")
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch {
		case r == '\n' || r == '\t':
			builder.WriteRune(' ')
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			continue
		default:
			builder.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func redactSecrets(value string) string {
	value = authorizationPattern.ReplaceAllString(strings.TrimSpace(value), "[secret]")
	return secretPattern.ReplaceAllString(value, "[secret]")
}

func boundedDisplayText(value string) string {
	if len(value) <= DisplayTextMaxBytes {
		return value
	}
	value = value[:DisplayTextMaxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return strings.TrimRight(value, " ")
}

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
	MutationFieldMaxBytes  = 256
	MutationPathMaxBytes   = 256
	MutationMaxPaths       = 8
)

var ErrInvalidFakeEvent = errors.New("fake history event contract is invalid")

// EventKind is the closed set of fake activity event kinds.
type EventKind string

const (
	EventKindPrompt   EventKind = "prompt"
	EventKindResponse EventKind = "response"
	EventKindCommand  EventKind = "command"
	EventKindRuntime  EventKind = "runtime"
	EventKindMutation EventKind = "mutation"
)

// FakeEvent is a bounded, display-safe record of fake activity.
type FakeEvent struct {
	SchemaVersion int             `json:"schema_version"`
	Kind          EventKind       `json:"kind"`
	EventID       string          `json:"event_id"`
	RunID         string          `json:"run_id"`
	SessionID     string          `json:"session_id"`
	Source        string          `json:"source"`
	Provenance    string          `json:"provenance"`
	DisplayText   string          `json:"display_text"`
	Mutation      *MutationRecord `json:"mutation,omitempty"`
	Undo          *UndoMetadata   `json:"undo,omitempty"`
}

// MutationRecord describes one inspected edit/write result.
type MutationRecord struct {
	ToolName              string   `json:"tool_name"`
	Status                string   `json:"status"`
	CommandSource         string   `json:"command_source"`
	RequestID             string   `json:"request_id,omitempty"`
	ApprovalID            string   `json:"approval_id,omitempty"`
	ApprovalAction        string   `json:"approval_action,omitempty"`
	ChangedPaths          []string `json:"changed_paths"`
	RequestedPath         string   `json:"requested_path,omitempty"`
	ExpectedEffect        string   `json:"expected_effect,omitempty"`
	PreviousVersion       string   `json:"previous_version,omitempty"`
	NewVersion            string   `json:"new_version,omitempty"`
	PreviousExists        bool     `json:"previous_exists"`
	BytesWritten          int      `json:"bytes_written,omitempty"`
	ReplacementCount      int      `json:"replacement_count,omitempty"`
	ResolvedPathAvailable bool     `json:"resolved_path_available"`
	ErrorKind             string   `json:"error_kind,omitempty"`
	ErrorMessage          string   `json:"error_message,omitempty"`
	DecisionRunID         string   `json:"decision_run_id,omitempty"`
	DecisionCapability    string   `json:"decision_capability,omitempty"`
}

// UndoMetadata records descriptive recovery metadata without executing undo.
type UndoMetadata struct {
	Available       bool     `json:"available"`
	Action          string   `json:"action,omitempty"`
	Paths           []string `json:"paths,omitempty"`
	PreviousVersion string   `json:"previous_version,omitempty"`
	NewVersion      string   `json:"new_version,omitempty"`
	Reason          string   `json:"reason,omitempty"`
}

// EventKinds returns the stable, closed event-kind set in display order.
func EventKinds() []EventKind {
	return []EventKind{EventKindPrompt, EventKindResponse, EventKindCommand, EventKindRuntime, EventKindMutation}
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
	if event.Kind == EventKindMutation {
		mutation, err := normalizeMutationRecord(event.Mutation)
		if err != nil {
			return FakeEvent{}, err
		}
		undo, err := normalizeUndoMetadata(event.Undo)
		if err != nil {
			return FakeEvent{}, err
		}
		event.Mutation = &mutation
		event.Undo = &undo
		return event, nil
	}
	if event.Mutation != nil || event.Undo != nil {
		return FakeEvent{}, fmt.Errorf("%w: non-mutation event has mutation metadata", ErrInvalidFakeEvent)
	}
	return event, nil
}

func normalizeMutationRecord(record *MutationRecord) (MutationRecord, error) {
	if record == nil {
		return MutationRecord{}, fmt.Errorf("%w: mutation record is required", ErrInvalidFakeEvent)
	}
	var normalized MutationRecord
	var err error
	if normalized.ToolName, err = normalizeRequiredField("mutation.tool_name", record.ToolName, SourceMaxBytes); err != nil {
		return MutationRecord{}, err
	}
	if normalized.Status, err = normalizeRequiredField("mutation.status", record.Status, SourceMaxBytes); err != nil {
		return MutationRecord{}, err
	}
	if normalized.CommandSource, err = normalizeRequiredField("mutation.command_source", record.CommandSource, SourceMaxBytes); err != nil {
		return MutationRecord{}, err
	}
	if normalized.ChangedPaths, err = normalizePathList("mutation.changed_paths", record.ChangedPaths, true); err != nil {
		return MutationRecord{}, err
	}
	if normalized.RequestID, err = normalizeOptionalField("mutation.request_id", record.RequestID, MutationFieldMaxBytes); err != nil {
		return MutationRecord{}, err
	}
	if normalized.ApprovalID, err = normalizeOptionalField("mutation.approval_id", record.ApprovalID, MutationFieldMaxBytes); err != nil {
		return MutationRecord{}, err
	}
	if normalized.ApprovalAction, err = normalizeOptionalField("mutation.approval_action", record.ApprovalAction, MutationFieldMaxBytes); err != nil {
		return MutationRecord{}, err
	}
	if normalized.RequestedPath, err = normalizeOptionalPath("mutation.requested_path", record.RequestedPath); err != nil {
		return MutationRecord{}, err
	}
	if normalized.ExpectedEffect, err = normalizeOptionalField("mutation.expected_effect", record.ExpectedEffect, MutationFieldMaxBytes); err != nil {
		return MutationRecord{}, err
	}
	if normalized.PreviousVersion, err = normalizeOptionalField("mutation.previous_version", record.PreviousVersion, MutationFieldMaxBytes); err != nil {
		return MutationRecord{}, err
	}
	if normalized.NewVersion, err = normalizeOptionalField("mutation.new_version", record.NewVersion, MutationFieldMaxBytes); err != nil {
		return MutationRecord{}, err
	}
	if normalized.ErrorKind, err = normalizeOptionalField("mutation.error_kind", record.ErrorKind, MutationFieldMaxBytes); err != nil {
		return MutationRecord{}, err
	}
	if normalized.ErrorMessage, err = normalizeOptionalField("mutation.error_message", record.ErrorMessage, MutationFieldMaxBytes); err != nil {
		return MutationRecord{}, err
	}
	if normalized.DecisionRunID, err = normalizeOptionalField("mutation.decision_run_id", record.DecisionRunID, MutationFieldMaxBytes); err != nil {
		return MutationRecord{}, err
	}
	if normalized.DecisionCapability, err = normalizeOptionalField("mutation.decision_capability", record.DecisionCapability, MutationFieldMaxBytes); err != nil {
		return MutationRecord{}, err
	}
	normalized.PreviousExists = record.PreviousExists
	normalized.BytesWritten = record.BytesWritten
	normalized.ReplacementCount = record.ReplacementCount
	normalized.ResolvedPathAvailable = record.ResolvedPathAvailable
	if normalized.BytesWritten < 0 || normalized.ReplacementCount < 0 {
		return MutationRecord{}, fmt.Errorf("%w: mutation numeric fields must be non-negative", ErrInvalidFakeEvent)
	}
	return normalized, nil
}

func normalizeUndoMetadata(metadata *UndoMetadata) (UndoMetadata, error) {
	if metadata == nil {
		return UndoMetadata{}, fmt.Errorf("%w: undo metadata is required", ErrInvalidFakeEvent)
	}
	var normalized UndoMetadata
	var err error
	normalized.Available = metadata.Available
	if normalized.Action, err = normalizeOptionalField("undo.action", metadata.Action, MutationFieldMaxBytes); err != nil {
		return UndoMetadata{}, err
	}
	if normalized.Paths, err = normalizePathList("undo.paths", metadata.Paths, metadata.Available); err != nil {
		return UndoMetadata{}, err
	}
	if normalized.PreviousVersion, err = normalizeOptionalField("undo.previous_version", metadata.PreviousVersion, MutationFieldMaxBytes); err != nil {
		return UndoMetadata{}, err
	}
	if normalized.NewVersion, err = normalizeOptionalField("undo.new_version", metadata.NewVersion, MutationFieldMaxBytes); err != nil {
		return UndoMetadata{}, err
	}
	if normalized.Reason, err = normalizeOptionalField("undo.reason", metadata.Reason, MutationFieldMaxBytes); err != nil {
		return UndoMetadata{}, err
	}
	if metadata.Available && normalized.Action == "" {
		return UndoMetadata{}, fmt.Errorf("%w: undo.action is required when undo is available", ErrInvalidFakeEvent)
	}
	if !metadata.Available && normalized.Reason == "" {
		return UndoMetadata{}, fmt.Errorf("%w: undo.reason is required when undo is unavailable", ErrInvalidFakeEvent)
	}
	return normalized, nil
}

func normalizeRequiredField(field string, value string, maxBytes int) (string, error) {
	normalized := boundedText(redactSecrets(stripTerminalControls(value)), maxBytes)
	if err := requiredBoundedString(field, normalized, maxBytes); err != nil {
		return "", err
	}
	return normalized, nil
}

func normalizeOptionalField(_ string, value string, maxBytes int) (string, error) {
	return boundedText(redactSecrets(stripTerminalControls(value)), maxBytes), nil
}

func normalizePathList(field string, paths []string, required bool) ([]string, error) {
	if len(paths) == 0 {
		if required {
			return nil, fmt.Errorf("%w: %s is empty", ErrInvalidFakeEvent, field)
		}
		return nil, nil
	}
	if len(paths) > MutationMaxPaths {
		return nil, fmt.Errorf("%w: %s exceeds %d paths", ErrInvalidFakeEvent, field, MutationMaxPaths)
	}
	normalized := make([]string, 0, len(paths))
	for index, path := range paths {
		value, err := normalizeRequiredPath(fmt.Sprintf("%s[%d]", field, index), path)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, value)
	}
	return normalized, nil
}

func normalizeOptionalPath(field string, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	return normalizeRequiredPath(field, path)
}

func normalizeRequiredPath(field string, path string) (string, error) {
	normalized, err := normalizeRequiredField(field, path, MutationPathMaxBytes)
	if err != nil {
		return "", err
	}
	if unsafeHistoryPath(normalized) {
		return "", fmt.Errorf("%w: %s must be workspace-relative", ErrInvalidFakeEvent, field)
	}
	return normalized, nil
}

func unsafeHistoryPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "." || strings.HasPrefix(path, "/") || strings.HasPrefix(path, "~") || strings.HasPrefix(path, "\\\\") || strings.Contains(path, ":\\") || strings.Contains(path, "$HOME") || strings.Contains(path, "${HOME}") || strings.Contains(strings.ToUpper(path), "XDG_") {
		return true
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." || part == ".aila" || part == ".agentera" {
			return true
		}
	}
	return false
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
	return boundedText(value, DisplayTextMaxBytes)
}

func boundedText(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return strings.TrimRight(value, " ")
}

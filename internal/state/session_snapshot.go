package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jgabor/aila/internal/diagnostic"
)

const (
	CurrentSessionSnapshotSchemaVersion = 1
	SessionIDMaxBytes                   = 128
	SnapshotStatusMaxBytes              = 64
	SnapshotSourceMaxBytes              = 64
	SnapshotDetailMaxBytes              = 512
	SnapshotResultMaxBytes              = 512
	SnapshotTextMaxBytes                = 4096
	SnapshotLabelMaxBytes               = 128
	SnapshotDiagnosticMaxBytes          = 512
	SnapshotBlockerMaxBytes             = 512
	SnapshotConcernMaxBytes             = 512
	SessionSnapshotMaxFileBytes         = 64 * 1024
)

var (
	ErrUnsafeSessionPath      = errors.New("session snapshot path is unsafe")
	ErrInvalidSessionSnapshot = errors.New("session snapshot contract is invalid")
)

// SessionSnapshotReadState reports whether current session memory was loaded or needs passive recovery.
type SessionSnapshotReadState string

const (
	SessionSnapshotNoMemory       SessionSnapshotReadState = "no_memory"
	SessionSnapshotLoaded         SessionSnapshotReadState = "loaded"
	SessionSnapshotRecoveryNeeded SessionSnapshotReadState = "recovery_needed"
)

// SessionSnapshotReadResult carries either a validated snapshot or a passive recovery diagnostic.
type SessionSnapshotReadResult struct {
	State       SessionSnapshotReadState
	Snapshot    SessionSnapshot
	Diagnostics []diagnostic.Diagnostic
}

// SessionSnapshotName identifies the single M16 current-session snapshot contract.
type SessionSnapshotName string

const CurrentSessionSnapshot SessionSnapshotName = "current_session_snapshot"

// SessionSnapshotProvenance records how the snapshot path was derived.
type SessionSnapshotProvenance struct {
	LogicalName   SessionSnapshotName
	WorkspaceRoot string
	StoreRoot     string
	RelativePath  string
}

// SessionSnapshotLocation is the store-owned path for the current session snapshot.
type SessionSnapshotLocation struct {
	Name       SessionSnapshotName
	Path       string
	Provenance SessionSnapshotProvenance
}

// SessionSnapshot is the M16 schema for visible current-session memory.
type SessionSnapshot struct {
	SchemaVersion int                          `json:"schema_version"`
	SessionID     string                       `json:"session_id"`
	Runtime       SessionSnapshotRuntime       `json:"runtime"`
	Active        bool                         `json:"active"`
	Transcript    []SessionSnapshotTurn        `json:"transcript_turns"`
	Queued        []SessionSnapshotQueuedEntry `json:"queued_entries"`
	Diagnostics   []SessionSnapshotDiagnostic  `json:"diagnostics"`
	Blockers      []SessionSnapshotBlocker     `json:"blockers"`
	Concerns      []SessionSnapshotConcern     `json:"concerns"`
}

// SessionSnapshotRuntime captures the current fake runtime status shown by the UI.
type SessionSnapshotRuntime struct {
	Status string `json:"status"`
	Source string `json:"source"`
	Detail string `json:"detail"`
	Result string `json:"result"`
}

// SessionSnapshotTurn captures a bounded visible transcript turn.
type SessionSnapshotTurn struct {
	Role   string `json:"role"`
	Source string `json:"source"`
	Text   string `json:"text"`
}

// SessionSnapshotQueuedEntry captures bounded queued input visible in current UI state.
type SessionSnapshotQueuedEntry struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Text   string `json:"text"`
}

// SessionSnapshotDiagnostic captures bounded visible diagnostic text.
type SessionSnapshotDiagnostic struct {
	Severity string `json:"severity"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

// SessionSnapshotBlocker captures bounded visible blocker text.
type SessionSnapshotBlocker struct {
	Source string `json:"source"`
	Text   string `json:"text"`
}

// SessionSnapshotConcern captures bounded visible concern text.
type SessionSnapshotConcern struct {
	Source string `json:"source"`
	Text   string `json:"text"`
}

// DescribeCurrentSessionSnapshot derives `.aila/sessions/current.json` from the workspace store layout.
func DescribeCurrentSessionSnapshot(workspacePath string) (SessionSnapshotLocation, error) {
	layout, err := DescribeStore(workspacePath)
	if err != nil {
		return SessionSnapshotLocation{}, err
	}
	return CurrentSessionSnapshotLocation(layout)
}

// CurrentSessionSnapshotLocation derives the current snapshot path from an existing store layout.
func CurrentSessionSnapshotLocation(layout Layout) (SessionSnapshotLocation, error) {
	const relativePath = "sessions/current.json"
	path := filepath.Clean(filepath.Join(layout.StoreRoot, relativePath))
	if err := ValidateCurrentSessionSnapshotPath(layout, path); err != nil {
		return SessionSnapshotLocation{}, err
	}
	return SessionSnapshotLocation{
		Name: CurrentSessionSnapshot,
		Path: path,
		Provenance: SessionSnapshotProvenance{
			LogicalName:   CurrentSessionSnapshot,
			WorkspaceRoot: layout.WorkspaceRoot,
			StoreRoot:     layout.StoreRoot,
			RelativePath:  relativePath,
		},
	}, nil
}

// ValidateCurrentSessionSnapshotPath accepts only the exact current snapshot path in the store layout.
func ValidateCurrentSessionSnapshotPath(layout Layout, requestedPath string) error {
	if strings.TrimSpace(requestedPath) == "" {
		return ErrUnsafeSessionPath
	}
	if containsParentTraversal(requestedPath) {
		return fmt.Errorf("%w: %s", ErrUnsafeSessionPath, requestedPath)
	}
	want := filepath.Clean(filepath.Join(layout.StoreRoot, "sessions", "current.json"))
	got := filepath.Clean(requestedPath)
	rel, err := filepath.Rel(layout.StoreRoot, got)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("%w: %s", ErrUnsafeSessionPath, requestedPath)
	}
	if got != want {
		return fmt.Errorf("%w: %s", ErrUnsafeSessionPath, requestedPath)
	}
	return nil
}

func containsParentTraversal(path string) bool {
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == ".." {
			return true
		}
	}
	return false
}

// ValidateSessionSnapshotContract validates the pure M16 schema bounds without reading or writing files.
func ValidateSessionSnapshotContract(snapshot SessionSnapshot) error {
	if snapshot.SchemaVersion != CurrentSessionSnapshotSchemaVersion {
		return fmt.Errorf("%w: unsupported schema_version %d", ErrInvalidSessionSnapshot, snapshot.SchemaVersion)
	}
	if err := boundedString("session_id", snapshot.SessionID, SessionIDMaxBytes); err != nil {
		return err
	}
	if err := validateRuntime(snapshot.Runtime); err != nil {
		return err
	}
	for index, turn := range snapshot.Transcript {
		if err := boundedString(fmt.Sprintf("transcript_turns[%d].role", index), turn.Role, SnapshotLabelMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("transcript_turns[%d].source", index), turn.Source, SnapshotSourceMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("transcript_turns[%d].text", index), turn.Text, SnapshotTextMaxBytes); err != nil {
			return err
		}
	}
	for index, entry := range snapshot.Queued {
		if err := boundedString(fmt.Sprintf("queued_entries[%d].id", index), entry.ID, SnapshotLabelMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("queued_entries[%d].source", index), entry.Source, SnapshotSourceMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("queued_entries[%d].text", index), entry.Text, SnapshotTextMaxBytes); err != nil {
			return err
		}
	}
	for index, item := range snapshot.Diagnostics {
		if err := boundedString(fmt.Sprintf("diagnostics[%d].severity", index), item.Severity, SnapshotLabelMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("diagnostics[%d].source", index), item.Source, SnapshotSourceMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("diagnostics[%d].message", index), item.Message, SnapshotDiagnosticMaxBytes); err != nil {
			return err
		}
	}
	for index, item := range snapshot.Blockers {
		if err := boundedString(fmt.Sprintf("blockers[%d].source", index), item.Source, SnapshotSourceMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("blockers[%d].text", index), item.Text, SnapshotBlockerMaxBytes); err != nil {
			return err
		}
	}
	for index, item := range snapshot.Concerns {
		if err := boundedString(fmt.Sprintf("concerns[%d].source", index), item.Source, SnapshotSourceMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("concerns[%d].text", index), item.Text, SnapshotConcernMaxBytes); err != nil {
			return err
		}
	}
	return nil
}

// ReadCurrentSessionSnapshot reads and validates `.aila/sessions/current.json` without repairing it.
func (s Store) ReadCurrentSessionSnapshot(ctx context.Context) (SessionSnapshotReadResult, error) {
	if err := ctx.Err(); err != nil {
		return SessionSnapshotReadResult{}, err
	}
	location, err := CurrentSessionSnapshotLocation(s.layout)
	if err != nil {
		return SessionSnapshotReadResult{}, err
	}
	if err := ValidateCurrentSessionSnapshotPath(s.layout, location.Path); err != nil {
		return SessionSnapshotReadResult{}, err
	}
	if err := validateCurrentSessionSnapshotComponents(s.layout, location.Path); err != nil {
		return snapshotRecoveryResult(err), nil
	}

	info, err := os.Lstat(location.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SessionSnapshotReadResult{State: SessionSnapshotNoMemory}, nil
		}
		return snapshotRecoveryResult(fmt.Errorf("stat current session snapshot: %w", err)), nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return snapshotRecoveryResult(fmt.Errorf("%w: current session snapshot is a symlink", ErrUnsafeSessionPath)), nil
	}
	if info.IsDir() {
		return snapshotRecoveryResult(fmt.Errorf("current session snapshot path is a directory")), nil
	}
	if info.Size() > SessionSnapshotMaxFileBytes {
		return snapshotRecoveryResult(fmt.Errorf("current session snapshot exceeds %d bytes", SessionSnapshotMaxFileBytes)), nil
	}

	file, err := os.Open(location.Path)
	if err != nil {
		return snapshotRecoveryResult(fmt.Errorf("open current session snapshot: %w", err)), nil
	}
	defer func() { _ = file.Close() }()

	decoder := json.NewDecoder(io.LimitReader(file, SessionSnapshotMaxFileBytes+1))
	decoder.DisallowUnknownFields()
	var snapshot SessionSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return snapshotRecoveryResult(fmt.Errorf("decode current session snapshot: %w", err)), nil
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return snapshotRecoveryResult(fmt.Errorf("decode current session snapshot: trailing data")), nil
	}
	if err := validateSessionSnapshotCompleteness(location.Path); err != nil {
		return snapshotRecoveryResult(err), nil
	}
	if err := ValidateSessionSnapshotContract(snapshot); err != nil {
		return snapshotRecoveryResult(err), nil
	}

	return SessionSnapshotReadResult{State: SessionSnapshotLoaded, Snapshot: snapshot}, nil
}

func validateSessionSnapshotCompleteness(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read current session snapshot: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(content, &fields); err != nil {
		return fmt.Errorf("decode current session snapshot fields: %w", err)
	}
	if err := requireJSONFields("current session snapshot", fields, []string{"schema_version", "session_id", "runtime", "active", "transcript_turns", "queued_entries", "diagnostics", "blockers", "concerns"}); err != nil {
		return err
	}
	var runtimeFields map[string]json.RawMessage
	if err := json.Unmarshal(fields["runtime"], &runtimeFields); err != nil {
		return fmt.Errorf("decode current session snapshot runtime fields: %w", err)
	}
	if err := requireJSONFields("current session snapshot runtime", runtimeFields, []string{"status", "source", "detail", "result"}); err != nil {
		return err
	}
	arrayChecks := []struct {
		field    string
		required []string
	}{
		{field: "transcript_turns", required: []string{"role", "source", "text"}},
		{field: "queued_entries", required: []string{"id", "source", "text"}},
		{field: "diagnostics", required: []string{"severity", "source", "message"}},
		{field: "blockers", required: []string{"source", "text"}},
		{field: "concerns", required: []string{"source", "text"}},
	}
	for _, check := range arrayChecks {
		if err := requireJSONArrayObjectFields(fields, check.field, check.required); err != nil {
			return err
		}
	}
	return nil
}

func requireJSONFields(label string, fields map[string]json.RawMessage, required []string) error {
	for _, field := range required {
		raw, ok := fields[field]
		if !ok {
			return fmt.Errorf("%s missing field %s", label, field)
		}
		if isJSONNull(raw) {
			return fmt.Errorf("%s null field %s", label, field)
		}
	}
	return nil
}

func requireJSONArrayObjectFields(fields map[string]json.RawMessage, field string, required []string) error {
	raw := fields[field]
	if isJSONNull(raw) {
		return fmt.Errorf("current session snapshot null field %s", field)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("decode current session snapshot %s fields: %w", field, err)
	}
	for index, item := range items {
		if err := requireJSONFields(fmt.Sprintf("current session snapshot %s[%d]", field, index), item, required); err != nil {
			return err
		}
	}
	return nil
}

func isJSONNull(raw json.RawMessage) bool {
	return string(raw) == "null"
}

// WriteCurrentSessionSnapshot validates, redacts, and atomically replaces `.aila/sessions/current.json`.
func (s Store) WriteCurrentSessionSnapshot(ctx context.Context, snapshot SessionSnapshot) (SessionSnapshotLocation, error) {
	if err := ctx.Err(); err != nil {
		return SessionSnapshotLocation{}, err
	}
	location, err := CurrentSessionSnapshotLocation(s.layout)
	if err != nil {
		return SessionSnapshotLocation{}, err
	}
	if err := ValidateCurrentSessionSnapshotPath(s.layout, location.Path); err != nil {
		return SessionSnapshotLocation{}, err
	}

	snapshot = redactSessionSnapshot(snapshot)
	if err := ValidateSessionSnapshotContract(snapshot); err != nil {
		return SessionSnapshotLocation{}, err
	}
	content, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return SessionSnapshotLocation{}, fmt.Errorf("encode current session snapshot: %w", err)
	}
	content = append(content, '\n')
	if len(content) > SessionSnapshotMaxFileBytes {
		return SessionSnapshotLocation{}, fmt.Errorf("%w: encoded snapshot exceeds %d bytes", ErrInvalidSessionSnapshot, SessionSnapshotMaxFileBytes)
	}

	if err := validateCurrentSessionSnapshotComponents(s.layout, location.Path); err != nil {
		return SessionSnapshotLocation{}, err
	}
	if err := ensureCurrentSessionSnapshotDirectory(s.layout, location.Path); err != nil {
		return SessionSnapshotLocation{}, fmt.Errorf("create current session snapshot directory: %w", err)
	}
	if err := validateCurrentSessionSnapshotComponents(s.layout, location.Path); err != nil {
		return SessionSnapshotLocation{}, err
	}
	if err := validateCurrentSessionSnapshotFinalFile(location.Path); err != nil {
		return SessionSnapshotLocation{}, err
	}
	if err := writeFileAtomic(ctx, location.Path, content); err != nil {
		return SessionSnapshotLocation{}, err
	}
	return location, nil
}

func validateCurrentSessionSnapshotComponents(layout Layout, path string) error {
	if err := ValidateCurrentSessionSnapshotPath(layout, path); err != nil {
		return err
	}
	for _, dir := range []string{layout.StoreRoot, filepath.Dir(path)} {
		info, err := os.Lstat(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("%w: inspect %s: %w", ErrUnsafeSessionPath, dir, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s is a symlink", ErrUnsafeSessionPath, dir)
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: %s is not a directory", ErrUnsafeSessionPath, dir)
		}
	}
	return nil
}

func ensureCurrentSessionSnapshotDirectory(layout Layout, path string) error {
	dir := filepath.Dir(path)
	if err := ValidateCurrentSessionSnapshotPath(layout, path); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}

func validateCurrentSessionSnapshotFinalFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("%w: inspect current session snapshot: %w", ErrUnsafeSessionPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: current session snapshot is a symlink", ErrUnsafeSessionPath)
	}
	if info.IsDir() {
		return fmt.Errorf("%w: current session snapshot is a directory", ErrUnsafeSessionPath)
	}
	return nil
}

func snapshotRecoveryResult(cause error) SessionSnapshotReadResult {
	return SessionSnapshotReadResult{
		State: SessionSnapshotRecoveryNeeded,
		Diagnostics: []diagnostic.Diagnostic{diagnostic.New(diagnostic.Spec{
			Category:         diagnostic.CategoryState,
			Source:           diagnostic.SourceStateOpen,
			Severity:         diagnostic.SeverityError,
			Message:          "current session snapshot requires recovery: " + cause.Error(),
			AffectedArtifact: diagnostic.ArtifactProjectStore,
			RecoveryAction:   diagnostic.RecoveryManualRepair,
			UserInputNeeded:  true,
		})},
	}
}

func redactSessionSnapshot(snapshot SessionSnapshot) SessionSnapshot {
	snapshot.SessionID = redactSnapshotText(snapshot.SessionID)
	snapshot.Runtime.Status = redactSnapshotText(snapshot.Runtime.Status)
	snapshot.Runtime.Source = redactSnapshotText(snapshot.Runtime.Source)
	snapshot.Runtime.Detail = redactSnapshotText(snapshot.Runtime.Detail)
	snapshot.Runtime.Result = redactSnapshotText(snapshot.Runtime.Result)
	for index := range snapshot.Transcript {
		snapshot.Transcript[index].Role = redactSnapshotText(snapshot.Transcript[index].Role)
		snapshot.Transcript[index].Source = redactSnapshotText(snapshot.Transcript[index].Source)
		snapshot.Transcript[index].Text = redactSnapshotText(snapshot.Transcript[index].Text)
	}
	for index := range snapshot.Queued {
		snapshot.Queued[index].ID = redactSnapshotText(snapshot.Queued[index].ID)
		snapshot.Queued[index].Source = redactSnapshotText(snapshot.Queued[index].Source)
		snapshot.Queued[index].Text = redactSnapshotText(snapshot.Queued[index].Text)
	}
	for index := range snapshot.Diagnostics {
		snapshot.Diagnostics[index].Severity = redactSnapshotText(snapshot.Diagnostics[index].Severity)
		snapshot.Diagnostics[index].Source = redactSnapshotText(snapshot.Diagnostics[index].Source)
		snapshot.Diagnostics[index].Message = redactSnapshotText(snapshot.Diagnostics[index].Message)
	}
	for index := range snapshot.Blockers {
		snapshot.Blockers[index].Source = redactSnapshotText(snapshot.Blockers[index].Source)
		snapshot.Blockers[index].Text = redactSnapshotText(snapshot.Blockers[index].Text)
	}
	for index := range snapshot.Concerns {
		snapshot.Concerns[index].Source = redactSnapshotText(snapshot.Concerns[index].Source)
		snapshot.Concerns[index].Text = redactSnapshotText(snapshot.Concerns[index].Text)
	}
	return snapshot
}

var (
	snapshotAuthorizationPattern = regexp.MustCompile(`(?i)\bauthorization\b\s*(?::|=|\s+)\s*(?:bearer|basic)?\s*\S+`)
	snapshotSecretPattern        = regexp.MustCompile(`(?i)\b(?:api[_-]?key|apikey|password|secret|token)\b\s*(?::|=|\s+)\s*\S+`)
)

func redactSnapshotText(value string) string {
	value = snapshotAuthorizationPattern.ReplaceAllString(strings.TrimSpace(value), "[secret]")
	return snapshotSecretPattern.ReplaceAllString(value, "[secret]")
}

func validateRuntime(runtime SessionSnapshotRuntime) error {
	checks := map[string]struct {
		value string
		limit int
	}{
		"runtime.status": {value: runtime.Status, limit: SnapshotStatusMaxBytes},
		"runtime.source": {value: runtime.Source, limit: SnapshotSourceMaxBytes},
		"runtime.detail": {value: runtime.Detail, limit: SnapshotDetailMaxBytes},
		"runtime.result": {value: runtime.Result, limit: SnapshotResultMaxBytes},
	}
	for field, check := range checks {
		if err := boundedString(field, check.value, check.limit); err != nil {
			return err
		}
	}
	return nil
}

func boundedString(field string, value string, maxBytes int) error {
	if len(value) > maxBytes {
		return fmt.Errorf("%w: %s exceeds %d bytes", ErrInvalidSessionSnapshot, field, maxBytes)
	}
	return nil
}

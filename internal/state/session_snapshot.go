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
	Run           *SessionSnapshotRun          `json:"run,omitempty"`
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

// SessionSnapshotRun captures a bounded non-interactive run summary.
type SessionSnapshotRun struct {
	Mode           string                          `json:"mode"`
	Prompt         string                          `json:"prompt"`
	Status         string                          `json:"status"`
	InspectedFiles []SessionSnapshotRunFile        `json:"inspected_files"`
	Commands       []SessionSnapshotRunCommand     `json:"commands"`
	ChangedFiles   []SessionSnapshotRunChangedFile `json:"changed_files,omitempty"`
	Mutation       *SessionSnapshotRunMutation     `json:"mutation,omitempty"`
	Blockers       []string                        `json:"blockers"`
	Caveats        []string                        `json:"caveats"`
	SourceRefs     []string                        `json:"source_refs"`
	StoredSession  bool                            `json:"stored_session"`
	StoredHistory  bool                            `json:"stored_history"`
}

// SessionSnapshotRunFile records one inspected file for a non-interactive run.
type SessionSnapshotRunFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	LineStart int    `json:"line_start,omitempty"`
	LineEnd   int    `json:"line_end,omitempty"`
	SourceRef string `json:"source_ref,omitempty"`
}

// SessionSnapshotRunCommand records one fixed command/check for a non-interactive run.
type SessionSnapshotRunCommand struct {
	Command  string `json:"command"`
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
	Summary  string `json:"summary,omitempty"`
}

// SessionSnapshotRunChangedFile records one changed file for a non-interactive write run.
type SessionSnapshotRunChangedFile struct {
	Path            string `json:"path"`
	Status          string `json:"status"`
	PreviousVersion string `json:"previous_version,omitempty"`
	NewVersion      string `json:"new_version,omitempty"`
	BytesWritten    int    `json:"bytes_written,omitempty"`
	SourceRef       string `json:"source_ref,omitempty"`
}

// SessionSnapshotRunMutation records bounded mutation evidence for a non-interactive write run.
type SessionSnapshotRunMutation struct {
	ToolName         string `json:"tool_name"`
	Status           string `json:"status"`
	Path             string `json:"path"`
	ExpectedEffect   string `json:"expected_effect,omitempty"`
	BytesWritten     int    `json:"bytes_written,omitempty"`
	ErrorKind        string `json:"error_kind,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
	DecisionSource   string `json:"decision_source,omitempty"`
	DecisionAutonomy string `json:"decision_autonomy,omitempty"`
	Allowed          bool   `json:"allowed"`
	Automatic        bool   `json:"automatic"`
	ApprovalRequired bool   `json:"approval_required"`
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
	if snapshot.Run != nil {
		if err := validateSessionSnapshotRun(*snapshot.Run); err != nil {
			return err
		}
	}
	return nil
}

func validateSessionSnapshotRun(run SessionSnapshotRun) error {
	if err := boundedString("run.mode", run.Mode, SnapshotLabelMaxBytes); err != nil {
		return err
	}
	if err := boundedString("run.prompt", run.Prompt, SnapshotTextMaxBytes); err != nil {
		return err
	}
	if err := boundedString("run.status", run.Status, SnapshotStatusMaxBytes); err != nil {
		return err
	}
	for index, file := range run.InspectedFiles {
		if err := boundedString(fmt.Sprintf("run.inspected_files[%d].path", index), file.Path, SnapshotLabelMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("run.inspected_files[%d].status", index), file.Status, SnapshotStatusMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("run.inspected_files[%d].source_ref", index), file.SourceRef, SnapshotTextMaxBytes); err != nil {
			return err
		}
	}
	for index, command := range run.Commands {
		if err := boundedString(fmt.Sprintf("run.commands[%d].command", index), command.Command, SnapshotTextMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("run.commands[%d].status", index), command.Status, SnapshotStatusMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("run.commands[%d].summary", index), command.Summary, SnapshotTextMaxBytes); err != nil {
			return err
		}
	}
	for index, file := range run.ChangedFiles {
		if err := boundedString(fmt.Sprintf("run.changed_files[%d].path", index), file.Path, SnapshotLabelMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("run.changed_files[%d].status", index), file.Status, SnapshotStatusMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("run.changed_files[%d].previous_version", index), file.PreviousVersion, SnapshotLabelMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("run.changed_files[%d].new_version", index), file.NewVersion, SnapshotLabelMaxBytes); err != nil {
			return err
		}
		if err := boundedString(fmt.Sprintf("run.changed_files[%d].source_ref", index), file.SourceRef, SnapshotTextMaxBytes); err != nil {
			return err
		}
	}
	if run.Mutation != nil {
		if err := validateSessionSnapshotRunMutation(*run.Mutation); err != nil {
			return err
		}
	}
	for index, blocker := range run.Blockers {
		if err := boundedString(fmt.Sprintf("run.blockers[%d]", index), blocker, SnapshotBlockerMaxBytes); err != nil {
			return err
		}
	}
	for index, caveat := range run.Caveats {
		if err := boundedString(fmt.Sprintf("run.caveats[%d]", index), caveat, SnapshotConcernMaxBytes); err != nil {
			return err
		}
	}
	for index, sourceRef := range run.SourceRefs {
		if err := boundedString(fmt.Sprintf("run.source_refs[%d]", index), sourceRef, SnapshotTextMaxBytes); err != nil {
			return err
		}
	}
	return nil
}

func validateSessionSnapshotRunMutation(mutation SessionSnapshotRunMutation) error {
	checks := []struct {
		name  string
		value string
		limit int
	}{
		{name: "run.mutation.tool_name", value: mutation.ToolName, limit: SnapshotLabelMaxBytes},
		{name: "run.mutation.status", value: mutation.Status, limit: SnapshotStatusMaxBytes},
		{name: "run.mutation.path", value: mutation.Path, limit: SnapshotLabelMaxBytes},
		{name: "run.mutation.expected_effect", value: mutation.ExpectedEffect, limit: SnapshotTextMaxBytes},
		{name: "run.mutation.error_kind", value: mutation.ErrorKind, limit: SnapshotLabelMaxBytes},
		{name: "run.mutation.error_message", value: mutation.ErrorMessage, limit: SnapshotTextMaxBytes},
		{name: "run.mutation.decision_source", value: mutation.DecisionSource, limit: SnapshotSourceMaxBytes},
		{name: "run.mutation.decision_autonomy", value: mutation.DecisionAutonomy, limit: SnapshotLabelMaxBytes},
	}
	for _, check := range checks {
		if err := boundedString(check.name, check.value, check.limit); err != nil {
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
	if rawRun, ok := fields["run"]; ok {
		if isJSONNull(rawRun) {
			return fmt.Errorf("current session snapshot null field run")
		}
		var runFields map[string]json.RawMessage
		if err := json.Unmarshal(rawRun, &runFields); err != nil {
			return fmt.Errorf("decode current session snapshot run fields: %w", err)
		}
		if err := requireJSONFields("current session snapshot run", runFields, []string{"mode", "prompt", "status", "inspected_files", "commands", "blockers", "caveats", "source_refs", "stored_session", "stored_history"}); err != nil {
			return err
		}
		if err := requireJSONArrayObjectFields(runFields, "inspected_files", []string{"path", "status"}); err != nil {
			return err
		}
		if err := requireJSONArrayObjectFields(runFields, "commands", []string{"command", "status", "exit_code"}); err != nil {
			return err
		}
		if _, ok := runFields["changed_files"]; ok {
			if err := requireJSONArrayObjectFields(runFields, "changed_files", []string{"path", "status"}); err != nil {
				return err
			}
		}
		if rawMutation, ok := runFields["mutation"]; ok {
			if isJSONNull(rawMutation) {
				return fmt.Errorf("current session snapshot null field run.mutation")
			}
			var mutationFields map[string]json.RawMessage
			if err := json.Unmarshal(rawMutation, &mutationFields); err != nil {
				return fmt.Errorf("decode current session snapshot run mutation fields: %w", err)
			}
			if err := requireJSONFields("current session snapshot run mutation", mutationFields, []string{"tool_name", "status", "path", "allowed", "automatic", "approval_required"}); err != nil {
				return err
			}
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

// ClearCurrentSessionSnapshot removes `.aila/sessions/current.json` through the same
// validated current-session path contract used by read and write. Missing memory is a no-op.
func (s Store) ClearCurrentSessionSnapshot(ctx context.Context) (SessionSnapshotLocation, error) {
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
	if err := validateCurrentSessionSnapshotComponents(s.layout, location.Path); err != nil {
		return SessionSnapshotLocation{}, err
	}
	if err := os.Remove(location.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return SessionSnapshotLocation{}, fmt.Errorf("clear current session snapshot: %w", err)
	}
	return location, nil
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
			Source:           diagnostic.SourceStateSnapshot,
			Severity:         diagnostic.SeverityError,
			Message:          "current session snapshot requires recovery: " + cause.Error(),
			AffectedArtifact: diagnostic.ArtifactSessionSnapshot,
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
	if snapshot.Run != nil {
		snapshot.Run.Mode = redactSnapshotText(snapshot.Run.Mode)
		snapshot.Run.Prompt = redactSnapshotText(snapshot.Run.Prompt)
		snapshot.Run.Status = redactSnapshotText(snapshot.Run.Status)
		for index := range snapshot.Run.InspectedFiles {
			snapshot.Run.InspectedFiles[index].Path = redactSnapshotText(snapshot.Run.InspectedFiles[index].Path)
			snapshot.Run.InspectedFiles[index].Status = redactSnapshotText(snapshot.Run.InspectedFiles[index].Status)
			snapshot.Run.InspectedFiles[index].SourceRef = redactSnapshotText(snapshot.Run.InspectedFiles[index].SourceRef)
		}
		for index := range snapshot.Run.Commands {
			snapshot.Run.Commands[index].Command = redactSnapshotText(snapshot.Run.Commands[index].Command)
			snapshot.Run.Commands[index].Status = redactSnapshotText(snapshot.Run.Commands[index].Status)
			snapshot.Run.Commands[index].Summary = redactSnapshotText(snapshot.Run.Commands[index].Summary)
		}
		for index := range snapshot.Run.ChangedFiles {
			snapshot.Run.ChangedFiles[index].Path = redactSnapshotText(snapshot.Run.ChangedFiles[index].Path)
			snapshot.Run.ChangedFiles[index].Status = redactSnapshotText(snapshot.Run.ChangedFiles[index].Status)
			snapshot.Run.ChangedFiles[index].PreviousVersion = redactSnapshotText(snapshot.Run.ChangedFiles[index].PreviousVersion)
			snapshot.Run.ChangedFiles[index].NewVersion = redactSnapshotText(snapshot.Run.ChangedFiles[index].NewVersion)
			snapshot.Run.ChangedFiles[index].SourceRef = redactSnapshotText(snapshot.Run.ChangedFiles[index].SourceRef)
		}
		if snapshot.Run.Mutation != nil {
			snapshot.Run.Mutation.ToolName = redactSnapshotText(snapshot.Run.Mutation.ToolName)
			snapshot.Run.Mutation.Status = redactSnapshotText(snapshot.Run.Mutation.Status)
			snapshot.Run.Mutation.Path = redactSnapshotText(snapshot.Run.Mutation.Path)
			snapshot.Run.Mutation.ExpectedEffect = redactSnapshotText(snapshot.Run.Mutation.ExpectedEffect)
			snapshot.Run.Mutation.ErrorKind = redactSnapshotText(snapshot.Run.Mutation.ErrorKind)
			snapshot.Run.Mutation.ErrorMessage = redactSnapshotText(snapshot.Run.Mutation.ErrorMessage)
			snapshot.Run.Mutation.DecisionSource = redactSnapshotText(snapshot.Run.Mutation.DecisionSource)
			snapshot.Run.Mutation.DecisionAutonomy = redactSnapshotText(snapshot.Run.Mutation.DecisionAutonomy)
		}
		for index := range snapshot.Run.Blockers {
			snapshot.Run.Blockers[index] = redactSnapshotText(snapshot.Run.Blockers[index])
		}
		for index := range snapshot.Run.Caveats {
			snapshot.Run.Caveats[index] = redactSnapshotText(snapshot.Run.Caveats[index])
		}
		for index := range snapshot.Run.SourceRefs {
			snapshot.Run.SourceRefs[index] = redactSnapshotText(snapshot.Run.SourceRefs[index])
		}
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

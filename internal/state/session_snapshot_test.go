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
)

func TestDescribeCurrentSessionSnapshotDerivesWorkspaceOwnedLocation(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	forbiddenRoots := []string{
		filepath.Join(t.TempDir(), "home"),
		filepath.Join(t.TempDir(), "xdg-state"),
		filepath.Join(t.TempDir(), "agentera"),
		filepath.Join(t.TempDir(), "tui"),
	}
	t.Setenv("HOME", forbiddenRoots[0])
	t.Setenv("XDG_STATE_HOME", forbiddenRoots[1])
	t.Setenv("AGENTERA_HOME", forbiddenRoots[2])
	t.Setenv("AILA_TUI_STATE", forbiddenRoots[3])

	location, err := DescribeCurrentSessionSnapshot(workspace)
	if err != nil {
		t.Fatalf("DescribeCurrentSessionSnapshot returned error: %v", err)
	}

	wantStore := filepath.Join(workspace, ".aila")
	wantPath := filepath.Join(wantStore, "sessions", "current.json")
	if location.Name != CurrentSessionSnapshot {
		t.Fatalf("snapshot name = %q, want %q", location.Name, CurrentSessionSnapshot)
	}
	if location.Path != wantPath {
		t.Fatalf("snapshot path = %q, want %q", location.Path, wantPath)
	}
	if location.Provenance.LogicalName != CurrentSessionSnapshot {
		t.Fatalf("provenance logical name = %q, want %q", location.Provenance.LogicalName, CurrentSessionSnapshot)
	}
	if location.Provenance.WorkspaceRoot != workspace {
		t.Fatalf("provenance workspace = %q, want %q", location.Provenance.WorkspaceRoot, workspace)
	}
	if location.Provenance.StoreRoot != wantStore {
		t.Fatalf("provenance store root = %q, want %q", location.Provenance.StoreRoot, wantStore)
	}
	if location.Provenance.RelativePath != "sessions/current.json" {
		t.Fatalf("provenance relative path = %q", location.Provenance.RelativePath)
	}

	for _, forbidden := range forbiddenRoots {
		for label, path := range map[string]string{
			"snapshot":  location.Path,
			"workspace": location.Provenance.WorkspaceRoot,
			"store":     location.Provenance.StoreRoot,
		} {
			if path == forbidden || strings.HasPrefix(path, forbidden+string(filepath.Separator)) {
				t.Fatalf("%s path %q was derived from forbidden state root %q", label, path, forbidden)
			}
		}
	}
}

func TestReadCurrentSessionSnapshotReportsNoMemoryWhenMissing(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	result, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("ReadCurrentSessionSnapshot returned error: %v", err)
	}
	if result.State != SessionSnapshotNoMemory {
		t.Fatalf("read state = %q, want %q", result.State, SessionSnapshotNoMemory)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("missing snapshot diagnostics = %v, want none", result.Diagnostics)
	}
}

func TestWriteAndReadCurrentSessionSnapshotRoundTripsBoundedRedactedSchema(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	snapshot := validSessionSnapshot()
	snapshot.SessionID = "session password=hunter2"
	snapshot.Runtime.Detail = "Authorization: Bearer sk-live-secret"
	snapshot.Transcript[0].Text = "hello token=abc123"
	snapshot.Queued[0].Text = "next api_key=snake-key"
	snapshot.Diagnostics[0].Message = "diagnostic secret=keep-out"

	location, err := store.WriteCurrentSessionSnapshot(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("WriteCurrentSessionSnapshot returned error: %v", err)
	}
	assertFileExists(t, location.Path)
	assertNoSessionSnapshotTempFiles(t, filepath.Dir(location.Path))

	result, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("ReadCurrentSessionSnapshot returned error: %v", err)
	}
	if result.State != SessionSnapshotLoaded {
		t.Fatalf("read state = %q, want %q", result.State, SessionSnapshotLoaded)
	}
	if result.Snapshot.SchemaVersion != CurrentSessionSnapshotSchemaVersion {
		t.Fatalf("schema version = %d", result.Snapshot.SchemaVersion)
	}
	joined := strings.Join([]string{
		result.Snapshot.SessionID,
		result.Snapshot.Runtime.Detail,
		result.Snapshot.Transcript[0].Text,
		result.Snapshot.Queued[0].Text,
		result.Snapshot.Diagnostics[0].Message,
	}, "\n")
	for _, leaked := range []string{"hunter2", "sk-live-secret", "abc123", "snake-key", "keep-out", "token=", "api_key="} {
		if strings.Contains(joined, leaked) {
			t.Fatalf("snapshot leaked %q in %q", leaked, joined)
		}
	}
	if strings.Count(joined, "[secret]") < 5 {
		t.Fatalf("snapshot content was not redacted enough: %q", joined)
	}
}

func TestClearCurrentSessionSnapshotRemovesOnlyCurrentMemory(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	location, err := store.WriteCurrentSessionSnapshot(context.Background(), validSessionSnapshot())
	if err != nil {
		t.Fatalf("WriteCurrentSessionSnapshot returned error: %v", err)
	}
	assertFileExists(t, location.Path)

	cleared, err := store.ClearCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("ClearCurrentSessionSnapshot returned error: %v", err)
	}
	if cleared.Path != location.Path {
		t.Fatalf("cleared path = %q, want %q", cleared.Path, location.Path)
	}
	if _, err := os.Stat(location.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("current snapshot after clear stat err = %v, want not exist", err)
	}
	assertStoreEntries(t, store.Layout().StoreRoot, []string{"artifacts/", "indexes/", "project.toml", "sessions/"})

	if _, err := store.ClearCurrentSessionSnapshot(context.Background()); err != nil {
		t.Fatalf("second ClearCurrentSessionSnapshot returned error: %v", err)
	}
	result, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("ReadCurrentSessionSnapshot returned error: %v", err)
	}
	if result.State != SessionSnapshotNoMemory {
		t.Fatalf("read state after clear = %q, want %q", result.State, SessionSnapshotNoMemory)
	}
}

func TestReadCurrentSessionSnapshotReturnsRecoveryForInvalidDataWithoutOverwrite(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"corrupt JSON":        `{"schema_version":`,
		"partial":             `{"schema_version":1,"session_id":"session-1"}`,
		"partial nested turn": strings.Replace(string(mustMarshalSnapshot(t, validSessionSnapshot())), `,"text":"hello"`, ``, 1),
		"null nested turn":    strings.Replace(string(mustMarshalSnapshot(t, validSessionSnapshot())), `"text":"hello"`, `"text":null`, 1),
		"version mismatch":    strings.Replace(string(mustMarshalSnapshot(t, validSessionSnapshot())), `"schema_version":1`, `"schema_version":2`, 1),
		"unknown field":       `{"schema_version":1,"session_id":"session-1","runtime":{"status":"idle","source":"fake","detail":"","result":""},"active":true,"transcript_turns":[],"queued_entries":[],"diagnostics":[],"blockers":[],"concerns":[],"unexpected":true}`,
		"oversized contract":  string(mustMarshalSnapshot(t, oversizedSessionSnapshot())),
	}
	for name, content := range tests {
		name, content := name, content
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
			location := mustCurrentSessionSnapshotLocation(t, store.Layout())
			seedCurrentSnapshot(t, location.Path, content)

			result, err := store.ReadCurrentSessionSnapshot(context.Background())
			if err != nil {
				t.Fatalf("ReadCurrentSessionSnapshot returned error: %v", err)
			}
			if result.State != SessionSnapshotRecoveryNeeded {
				t.Fatalf("read state = %q, want %q", result.State, SessionSnapshotRecoveryNeeded)
			}
			if len(result.Diagnostics) != 1 || !result.Diagnostics[0].UserInputNeeded {
				t.Fatalf("diagnostics = %#v, want one actionable recovery diagnostic", result.Diagnostics)
			}
			assertFileContent(t, location.Path, content)
			assertStoreEntries(t, store.Layout().StoreRoot, []string{"artifacts/", "indexes/", "project.toml", "sessions/"})
		})
	}
}

func TestReadCurrentSessionSnapshotReturnsRecoveryForOversizedFileWithoutOverwrite(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	location := mustCurrentSessionSnapshotLocation(t, store.Layout())
	content := strings.Repeat("x", SessionSnapshotMaxFileBytes+1)
	seedCurrentSnapshot(t, location.Path, content)

	result, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("ReadCurrentSessionSnapshot returned error: %v", err)
	}
	if result.State != SessionSnapshotRecoveryNeeded {
		t.Fatalf("read state = %q, want %q", result.State, SessionSnapshotRecoveryNeeded)
	}
	assertFileContent(t, location.Path, content)
}

func TestReadCurrentSessionSnapshotRejectsSymlinkedSessionDirectory(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	location := mustCurrentSessionSnapshotLocation(t, store.Layout())
	externalDir := filepath.Join(t.TempDir(), "external")
	if err := os.MkdirAll(externalDir, 0o755); err != nil {
		t.Fatalf("create external dir: %v", err)
	}
	if err := os.Symlink(externalDir, filepath.Dir(location.Path)); err != nil {
		t.Fatalf("create sessions symlink: %v", err)
	}
	seedCurrentSnapshot(t, filepath.Join(externalDir, "current.json"), string(mustMarshalSnapshot(t, validSessionSnapshot())))

	result, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("ReadCurrentSessionSnapshot returned error: %v", err)
	}
	if result.State != SessionSnapshotRecoveryNeeded {
		t.Fatalf("read state = %q, want %q", result.State, SessionSnapshotRecoveryNeeded)
	}
	assertFileContent(t, filepath.Join(externalDir, "current.json"), string(mustMarshalSnapshot(t, validSessionSnapshot())))
}

func TestWriteCurrentSessionSnapshotRejectsInvalidWithoutOverwriteOrTempFile(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	location := mustCurrentSessionSnapshotLocation(t, store.Layout())
	const existing = `{"schema_version":1,"keep":true}`
	seedCurrentSnapshot(t, location.Path, existing)

	invalid := validSessionSnapshot()
	invalid.Runtime.Result = strings.Repeat("x", SnapshotResultMaxBytes+1)
	if _, err := store.WriteCurrentSessionSnapshot(context.Background(), invalid); !errors.Is(err, ErrInvalidSessionSnapshot) {
		t.Fatalf("invalid write error = %v, want ErrInvalidSessionSnapshot", err)
	}
	assertFileContent(t, location.Path, existing)
	assertNoSessionSnapshotTempFiles(t, filepath.Dir(location.Path))
}

func TestWriteCurrentSessionSnapshotRejectsSymlinkedSessionDirectory(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	location := mustCurrentSessionSnapshotLocation(t, store.Layout())
	externalDir := filepath.Join(t.TempDir(), "external")
	if err := os.MkdirAll(externalDir, 0o755); err != nil {
		t.Fatalf("create external dir: %v", err)
	}
	if err := os.Symlink(externalDir, filepath.Dir(location.Path)); err != nil {
		t.Fatalf("create sessions symlink: %v", err)
	}

	if _, err := store.WriteCurrentSessionSnapshot(context.Background(), validSessionSnapshot()); !errors.Is(err, ErrUnsafeSessionPath) {
		t.Fatalf("symlinked sessions write error = %v, want ErrUnsafeSessionPath", err)
	}
	if _, err := os.Stat(filepath.Join(externalDir, "current.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external current snapshot stat error = %v, want not exist", err)
	}
	assertNoSessionSnapshotTempFiles(t, externalDir)
}

func TestWriteCurrentSessionSnapshotCreatesOnlySessionsDirectoryAndAtomicFinalContent(t *testing.T) {
	t.Parallel()

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	location := mustCurrentSessionSnapshotLocation(t, store.Layout())
	if _, err := os.Stat(filepath.Dir(location.Path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sessions directory exists before write or stat error = %v", err)
	}

	if _, err := store.WriteCurrentSessionSnapshot(context.Background(), validSessionSnapshot()); err != nil {
		t.Fatalf("WriteCurrentSessionSnapshot returned error: %v", err)
	}
	assertFileExists(t, location.Path)
	assertNoSessionSnapshotTempFiles(t, filepath.Dir(location.Path))
	assertStoreEntries(t, store.Layout().StoreRoot, []string{"artifacts/", "indexes/", "project.toml", "sessions/"})

	content, err := os.ReadFile(location.Path)
	if err != nil {
		t.Fatalf("read current snapshot: %v", err)
	}
	var decoded SessionSnapshot
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("current snapshot is not final JSON content: %v", err)
	}
	if !reflect.DeepEqual(decoded, validSessionSnapshot()) {
		t.Fatalf("decoded snapshot = %#v, want %#v", decoded, validSessionSnapshot())
	}
}

func TestSessionSnapshotSchemaIncludesCurrentUIStateFields(t *testing.T) {
	t.Parallel()

	snapshot := validSessionSnapshot()
	content, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}

	for _, field := range []string{
		"schema_version",
		"session_id",
		"runtime",
		"active",
		"transcript_turns",
		"queued_entries",
		"diagnostics",
		"blockers",
		"concerns",
	} {
		if _, ok := decoded[field]; !ok {
			t.Fatalf("snapshot JSON missing field %q in %s", field, content)
		}
	}

	runtime, ok := decoded["runtime"].(map[string]any)
	if !ok {
		t.Fatalf("runtime field has type %T", decoded["runtime"])
	}
	for _, field := range []string{"status", "source", "detail", "result"} {
		if _, ok := runtime[field]; !ok {
			t.Fatalf("runtime JSON missing field %q in %s", field, content)
		}
	}
}

func TestValidateSessionSnapshotContractAcceptsBoundedVisibleState(t *testing.T) {
	t.Parallel()

	if err := ValidateSessionSnapshotContract(validSessionSnapshot()); err != nil {
		t.Fatalf("ValidateSessionSnapshotContract returned error: %v", err)
	}
}

func TestValidateSessionSnapshotContractAcceptsBoundedRunMemory(t *testing.T) {
	t.Parallel()

	snapshot := validSessionSnapshot()
	snapshot.Run = validSessionSnapshotRun()
	if err := ValidateSessionSnapshotContract(snapshot); err != nil {
		t.Fatalf("ValidateSessionSnapshotContract with run memory returned error: %v", err)
	}

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	if _, err := store.WriteCurrentSessionSnapshot(context.Background(), snapshot); err != nil {
		t.Fatalf("WriteCurrentSessionSnapshot with run memory returned error: %v", err)
	}
	result, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("ReadCurrentSessionSnapshot with run memory returned error: %v", err)
	}
	if result.State != SessionSnapshotLoaded || result.Snapshot.Run == nil {
		t.Fatalf("read run memory state = %q run=%#v, want loaded run memory", result.State, result.Snapshot.Run)
	}
	if result.Snapshot.Run.Prompt != "explain repo" || len(result.Snapshot.Run.InspectedFiles) != 1 || len(result.Snapshot.Run.Commands) != 1 {
		t.Fatalf("run memory = %#v", result.Snapshot.Run)
	}
}

func TestValidateSessionSnapshotContractAcceptsBoundedWriteRunMemory(t *testing.T) {
	t.Parallel()

	snapshot := validSessionSnapshot()
	snapshot.Run = validSessionSnapshotWriteRun()
	if err := ValidateSessionSnapshotContract(snapshot); err != nil {
		t.Fatalf("ValidateSessionSnapshotContract with write run memory returned error: %v", err)
	}

	store := mustOpenProjectStore(t, filepath.Join(t.TempDir(), "workspace"))
	if _, err := store.WriteCurrentSessionSnapshot(context.Background(), snapshot); err != nil {
		t.Fatalf("WriteCurrentSessionSnapshot with write run memory returned error: %v", err)
	}
	result, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("ReadCurrentSessionSnapshot with write run memory returned error: %v", err)
	}
	run := result.Snapshot.Run
	if result.State != SessionSnapshotLoaded || run == nil {
		t.Fatalf("read write run memory state = %q run=%#v, want loaded write run memory", result.State, run)
	}
	if run.Mode != "non_interactive_write" || len(run.ChangedFiles) != 1 || run.Mutation == nil {
		t.Fatalf("write run memory = %#v", run)
	}
	if run.Mutation.ToolName != "write" || run.Mutation.Status != "completed" || run.Mutation.DecisionAutonomy != "write" || !run.Mutation.Allowed || !run.Mutation.Automatic || run.Mutation.ApprovalRequired {
		t.Fatalf("write run mutation = %#v", run.Mutation)
	}
}

func TestValidateSessionSnapshotContractRejectsUnsupportedVersionAndOversizedStrings(t *testing.T) {
	t.Parallel()

	versionMismatch := validSessionSnapshot()
	versionMismatch.SchemaVersion = CurrentSessionSnapshotSchemaVersion + 1
	if err := ValidateSessionSnapshotContract(versionMismatch); !errors.Is(err, ErrInvalidSessionSnapshot) {
		t.Fatalf("version mismatch error = %v, want ErrInvalidSessionSnapshot", err)
	}

	tests := map[string]func(*SessionSnapshot){
		"session id": func(snapshot *SessionSnapshot) { snapshot.SessionID = strings.Repeat("s", SessionIDMaxBytes+1) },
		"runtime status": func(snapshot *SessionSnapshot) {
			snapshot.Runtime.Status = strings.Repeat("s", SnapshotStatusMaxBytes+1)
		},
		"runtime source": func(snapshot *SessionSnapshot) {
			snapshot.Runtime.Source = strings.Repeat("s", SnapshotSourceMaxBytes+1)
		},
		"runtime detail": func(snapshot *SessionSnapshot) {
			snapshot.Runtime.Detail = strings.Repeat("s", SnapshotDetailMaxBytes+1)
		},
		"runtime result": func(snapshot *SessionSnapshot) {
			snapshot.Runtime.Result = strings.Repeat("s", SnapshotResultMaxBytes+1)
		},
		"turn role": func(snapshot *SessionSnapshot) {
			snapshot.Transcript[0].Role = strings.Repeat("s", SnapshotLabelMaxBytes+1)
		},
		"turn text": func(snapshot *SessionSnapshot) {
			snapshot.Transcript[0].Text = strings.Repeat("s", SnapshotTextMaxBytes+1)
		},
		"queued id": func(snapshot *SessionSnapshot) { snapshot.Queued[0].ID = strings.Repeat("s", SnapshotLabelMaxBytes+1) },
		"diagnostic text": func(snapshot *SessionSnapshot) {
			snapshot.Diagnostics[0].Message = strings.Repeat("s", SnapshotDiagnosticMaxBytes+1)
		},
		"blocker text": func(snapshot *SessionSnapshot) {
			snapshot.Blockers[0].Text = strings.Repeat("s", SnapshotBlockerMaxBytes+1)
		},
		"concern text": func(snapshot *SessionSnapshot) {
			snapshot.Concerns[0].Text = strings.Repeat("s", SnapshotConcernMaxBytes+1)
		},
		"run prompt": func(snapshot *SessionSnapshot) {
			snapshot.Run = validSessionSnapshotRun()
			snapshot.Run.Prompt = strings.Repeat("s", SnapshotTextMaxBytes+1)
		},
		"run file path": func(snapshot *SessionSnapshot) {
			snapshot.Run = validSessionSnapshotRun()
			snapshot.Run.InspectedFiles[0].Path = strings.Repeat("s", SnapshotLabelMaxBytes+1)
		},
		"run command summary": func(snapshot *SessionSnapshot) {
			snapshot.Run = validSessionSnapshotRun()
			snapshot.Run.Commands[0].Summary = strings.Repeat("s", SnapshotTextMaxBytes+1)
		},
		"run changed file path": func(snapshot *SessionSnapshot) {
			snapshot.Run = validSessionSnapshotWriteRun()
			snapshot.Run.ChangedFiles[0].Path = strings.Repeat("s", SnapshotLabelMaxBytes+1)
		},
		"run mutation path": func(snapshot *SessionSnapshot) {
			snapshot.Run = validSessionSnapshotWriteRun()
			snapshot.Run.Mutation.Path = strings.Repeat("s", SnapshotLabelMaxBytes+1)
		},
		"run mutation expected effect": func(snapshot *SessionSnapshot) {
			snapshot.Run = validSessionSnapshotWriteRun()
			snapshot.Run.Mutation.ExpectedEffect = strings.Repeat("s", SnapshotTextMaxBytes+1)
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			snapshot := validSessionSnapshot()
			mutate(&snapshot)
			if err := ValidateSessionSnapshotContract(snapshot); !errors.Is(err, ErrInvalidSessionSnapshot) {
				t.Fatalf("oversized %s error = %v, want ErrInvalidSessionSnapshot", name, err)
			}
		})
	}
}

func TestValidateCurrentSessionSnapshotPathRejectsUnsafeAndEscapedPaths(t *testing.T) {
	t.Parallel()

	layout := mustDescribeStore(t, filepath.Join(t.TempDir(), "workspace"))
	location, err := CurrentSessionSnapshotLocation(layout)
	if err != nil {
		t.Fatalf("CurrentSessionSnapshotLocation returned error: %v", err)
	}
	if err := ValidateCurrentSessionSnapshotPath(layout, location.Path); err != nil {
		t.Fatalf("valid current path rejected: %v", err)
	}

	unsafePaths := []string{
		"",
		layout.StoreRoot + string(filepath.Separator) + ".." + string(filepath.Separator) + ".aila" + string(filepath.Separator) + "sessions" + string(filepath.Separator) + "current.json",
		filepath.Join(layout.StoreRoot, "sessions", "..", "project.toml"),
		filepath.Join(layout.StoreRoot, "sessions", "current.json", "extra"),
		filepath.Join(layout.StoreRoot, "sessions", "other.json"),
		filepath.Join(layout.StoreRoot, "..", "current.json"),
		filepath.Join(t.TempDir(), "workspace", ".aila", "sessions", "current.json"),
	}
	for _, path := range unsafePaths {
		if err := ValidateCurrentSessionSnapshotPath(layout, path); !errors.Is(err, ErrUnsafeSessionPath) {
			t.Fatalf("unsafe path %q error = %v, want ErrUnsafeSessionPath", path, err)
		}
	}
}

func TestSessionSnapshotContractFieldsRemainNarrow(t *testing.T) {
	t.Parallel()

	want := []string{
		"SchemaVersion",
		"SessionID",
		"Runtime",
		"Active",
		"Transcript",
		"Queued",
		"Diagnostics",
		"Blockers",
		"Concerns",
		"Run",
	}
	snapshotType := reflect.TypeOf(SessionSnapshot{})
	got := make([]string, 0, snapshotType.NumField())
	for index := 0; index < snapshotType.NumField(); index++ {
		got = append(got, snapshotType.Field(index).Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SessionSnapshot fields = %v, want %v", got, want)
	}
}

func validSessionSnapshot() SessionSnapshot {
	return SessionSnapshot{
		SchemaVersion: CurrentSessionSnapshotSchemaVersion,
		SessionID:     "session-1",
		Runtime: SessionSnapshotRuntime{
			Status: "idle",
			Source: "fake-runtime",
			Detail: "waiting for input",
			Result: "none",
		},
		Active: true,
		Transcript: []SessionSnapshotTurn{{
			Role:   "user",
			Source: "prompt",
			Text:   "hello",
		}},
		Queued: []SessionSnapshotQueuedEntry{{
			ID:     "queue-1",
			Source: "prompt",
			Text:   "next",
		}},
		Diagnostics: []SessionSnapshotDiagnostic{{
			Severity: "warning",
			Source:   "state",
			Message:  "bounded diagnostic",
		}},
		Blockers: []SessionSnapshotBlocker{{
			Source: "display",
			Text:   "visible blocker",
		}},
		Concerns: []SessionSnapshotConcern{{
			Source: "display",
			Text:   "visible concern",
		}},
	}
}

func validSessionSnapshotRun() *SessionSnapshotRun {
	return &SessionSnapshotRun{
		Mode:   "non_interactive_read_only",
		Prompt: "explain repo",
		Status: "flagged",
		InspectedFiles: []SessionSnapshotRunFile{{
			Path:      "README.md",
			Status:    "completed",
			LineStart: 1,
			LineEnd:   20,
			SourceRef: "README.md:1-20",
		}},
		Commands: []SessionSnapshotRunCommand{{
			Command:  "git status --short --branch",
			Status:   "completed",
			ExitCode: 0,
			Summary:  "## main",
		}},
		Blockers:      []string{},
		Caveats:       []string{"provider model execution deferred"},
		SourceRefs:    []string{"README.md:1-20", "git status --short --branch"},
		StoredSession: true,
		StoredHistory: true,
	}
}

func validSessionSnapshotWriteRun() *SessionSnapshotRun {
	return &SessionSnapshotRun{
		Mode:   "non_interactive_write",
		Prompt: "create a note",
		Status: "flagged",
		InspectedFiles: []SessionSnapshotRunFile{{
			Path:      "README.md",
			Status:    "completed",
			LineStart: 1,
			LineEnd:   20,
			SourceRef: "README.md:1-20",
		}},
		Commands: []SessionSnapshotRunCommand{{
			Command:  "git status --short --branch",
			Status:   "completed",
			ExitCode: 0,
			Summary:  "## main",
		}},
		ChangedFiles: []SessionSnapshotRunChangedFile{{
			Path:            "docs/aila-run-output.md",
			Status:          "completed",
			PreviousVersion: "missing",
			NewVersion:      "sha256:write-run",
			BytesWritten:    120,
			SourceRef:       "docs/aila-run-output.md",
		}},
		Mutation: &SessionSnapshotRunMutation{
			ToolName:         "write",
			Status:           "completed",
			Path:             "docs/aila-run-output.md",
			ExpectedEffect:   "create bounded non-interactive run output",
			BytesWritten:     120,
			DecisionSource:   "autonomy_policy",
			DecisionAutonomy: "write",
			Allowed:          true,
			Automatic:        true,
			ApprovalRequired: false,
		},
		Blockers:      []string{},
		Caveats:       []string{"deterministic write run; provider model execution deferred"},
		SourceRefs:    []string{"README.md:1-20", "docs/aila-run-output.md", "git status --short --branch"},
		StoredSession: true,
		StoredHistory: true,
	}
}

func oversizedSessionSnapshot() SessionSnapshot {
	snapshot := validSessionSnapshot()
	snapshot.Runtime.Result = strings.Repeat("x", SnapshotResultMaxBytes+1)
	return snapshot
}

func mustCurrentSessionSnapshotLocation(t *testing.T, layout Layout) SessionSnapshotLocation {
	t.Helper()
	location, err := CurrentSessionSnapshotLocation(layout)
	if err != nil {
		t.Fatalf("CurrentSessionSnapshotLocation returned error: %v", err)
	}
	return location
}

func seedCurrentSnapshot(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create session snapshot directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seed current snapshot: %v", err)
	}
}

func mustMarshalSnapshot(t *testing.T, snapshot SessionSnapshot) []byte {
	t.Helper()
	content, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return content
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("%s is a directory, want file", path)
	}
}

func assertNoSessionSnapshotTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read snapshot dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".current.json.") && strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("leftover temp file %s", filepath.Join(dir, entry.Name()))
		}
	}
}

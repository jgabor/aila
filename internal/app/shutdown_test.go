package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/diagnostic"
)

func TestShutdownErrorIncludesRecoveryNeededStateWithoutOverwritingMetadata(t *testing.T) {
	workspace := t.TempDir()
	projectDir := filepath.Join(workspace, ".aila")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create project dir: %v", err)
	}
	projectFile := filepath.Join(projectDir, "project.toml")
	corrupt := "schema_version = not-a-number\n"
	if err := os.WriteFile(projectFile, []byte(corrupt), 0o644); err != nil {
		t.Fatalf("write corrupt project metadata: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	state, err := initialDisplayState(context.Background(), workspace)
	if err != nil {
		t.Fatalf("initial display state: %v", err)
	}
	shutdown := NewShutdownError(mergeShutdownDiagnostics(state.Diagnostics, []diagnostic.Diagnostic{signalShutdownDiagnostic(context.Canceled)}))

	if !strings.Contains(shutdown.Error(), "project metadata requires recovery") {
		t.Fatalf("shutdown error = %q, want recovery-needed state", shutdown.Error())
	}
	if !strings.Contains(shutdown.Error(), "signal-triggered shutdown requested") {
		t.Fatalf("shutdown error = %q, want signal shutdown diagnostic", shutdown.Error())
	}
	if after, err := os.ReadFile(projectFile); err != nil || string(after) != corrupt {
		t.Fatalf("project metadata changed or unreadable: content=%q err=%v", string(after), err)
	}
}

func TestShutdownErrorIncludesDegradedStoreOpenFailureWithoutOverwritingMetadata(t *testing.T) {
	workspace := t.TempDir()
	layout := mustStoreLayout(t, workspace)
	if err := os.MkdirAll(layout.ProjectFile, 0o755); err != nil {
		t.Fatalf("create project metadata directory: %v", err)
	}
	sentinel := filepath.Join(layout.ProjectFile, "preserve.txt")
	if err := os.WriteFile(sentinel, []byte("keep\n"), 0o644); err != nil {
		t.Fatalf("write sentinel metadata content: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	state, err := initialDisplayState(context.Background(), workspace)
	if err != nil {
		t.Fatalf("initial display state: %v", err)
	}
	shutdown := NewShutdownError(mergeShutdownDiagnostics(state.Diagnostics, []diagnostic.Diagnostic{signalShutdownDiagnostic(context.Canceled)}))

	if state.ProjectStoreStatus != "degraded" {
		t.Fatalf("project store status = %q, want degraded", state.ProjectStoreStatus)
	}
	for _, token := range []string{"project store unavailable", "project metadata path is a directory", "signal-triggered shutdown requested"} {
		if !strings.Contains(shutdown.Error(), token) {
			t.Fatalf("shutdown error = %q, want token %q", shutdown.Error(), token)
		}
	}
	if strings.Contains(shutdown.Error(), workspace) || strings.Contains(shutdown.Error(), layout.ProjectFile) {
		t.Fatalf("shutdown error leaked path: %q", shutdown.Error())
	}
	if len(shutdown.Diagnostics) != 2 {
		t.Fatalf("shutdown diagnostics length = %d, want degraded store and signal diagnostics", len(shutdown.Diagnostics))
	}
	got := shutdown.Diagnostics[0]
	if got.Source != diagnostic.SourceStateOpen || got.Severity != diagnostic.SeverityError || got.AffectedArtifact != diagnostic.ArtifactProjectStore || got.RecoveryAction != diagnostic.RecoveryInspect || !got.UserInputNeeded {
		t.Fatalf("shutdown degraded diagnostic = %#v", got)
	}
	if after, err := os.ReadFile(sentinel); err != nil || string(after) != "keep\n" {
		t.Fatalf("metadata directory content changed or unreadable: content=%q err=%v", string(after), err)
	}
	if info, err := os.Stat(layout.ProjectFile); err != nil || !info.IsDir() {
		t.Fatalf("project metadata unavailable state overwritten: info=%#v err=%v", info, err)
	}
}

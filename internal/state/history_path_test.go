package state

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestDescribeFakeHistoryDerivesWorkspaceOwnedLocation(t *testing.T) {
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

	location, err := DescribeFakeHistory(workspace)
	if err != nil {
		t.Fatalf("DescribeFakeHistory returned error: %v", err)
	}

	wantStore := filepath.Join(workspace, ".aila")
	wantPath := filepath.Join(wantStore, "history", "fake-events.jsonl")
	if location.Name != CurrentFakeHistory {
		t.Fatalf("history name = %q, want %q", location.Name, CurrentFakeHistory)
	}
	if location.Path != wantPath {
		t.Fatalf("history path = %q, want %q", location.Path, wantPath)
	}
	if location.Provenance.LogicalName != CurrentFakeHistory {
		t.Fatalf("provenance logical name = %q, want %q", location.Provenance.LogicalName, CurrentFakeHistory)
	}
	if location.Provenance.WorkspaceRoot != workspace {
		t.Fatalf("provenance workspace = %q, want %q", location.Provenance.WorkspaceRoot, workspace)
	}
	if location.Provenance.StoreRoot != wantStore {
		t.Fatalf("provenance store root = %q, want %q", location.Provenance.StoreRoot, wantStore)
	}
	if location.Provenance.RelativePath != "history/fake-events.jsonl" {
		t.Fatalf("provenance relative path = %q", location.Provenance.RelativePath)
	}

	for _, forbidden := range forbiddenRoots {
		for label, path := range map[string]string{
			"history":   location.Path,
			"workspace": location.Provenance.WorkspaceRoot,
			"store":     location.Provenance.StoreRoot,
		} {
			if path == forbidden || strings.HasPrefix(path, forbidden+string(filepath.Separator)) {
				t.Fatalf("%s path %q was derived from forbidden state root %q", label, path, forbidden)
			}
		}
	}
}

func TestValidateFakeHistoryPathRejectsUnsafeAndEscapedPaths(t *testing.T) {
	t.Parallel()

	layout := mustDescribeStore(t, filepath.Join(t.TempDir(), "workspace"))
	location, err := CurrentFakeHistoryLocation(layout)
	if err != nil {
		t.Fatalf("CurrentFakeHistoryLocation returned error: %v", err)
	}
	if err := ValidateFakeHistoryPath(layout, location.Path); err != nil {
		t.Fatalf("valid fake history path rejected: %v", err)
	}

	unsafePaths := []string{
		"",
		layout.StoreRoot + string(filepath.Separator) + ".." + string(filepath.Separator) + ".aila" + string(filepath.Separator) + "history" + string(filepath.Separator) + "fake-events.jsonl",
		filepath.Join(layout.StoreRoot, "history", "..", "project.toml"),
		filepath.Join(layout.StoreRoot, "history", "fake-events.jsonl", "extra"),
		filepath.Join(layout.StoreRoot, "history", "other.jsonl"),
		filepath.Join(layout.StoreRoot, "..", "fake-events.jsonl"),
		filepath.Join(t.TempDir(), "workspace", ".aila", "history", "fake-events.jsonl"),
	}
	for _, path := range unsafePaths {
		if err := ValidateFakeHistoryPath(layout, path); !errors.Is(err, ErrUnsafeFakeHistoryPath) {
			t.Fatalf("unsafe path %q error = %v, want ErrUnsafeFakeHistoryPath", path, err)
		}
	}
}

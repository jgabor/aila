package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
)

func TestInitialDisplayStateInitializesProjectStoreThroughApp(t *testing.T) {
	configHome := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	view, err := initialDisplayState(context.Background(), workspace)
	if err != nil {
		t.Fatalf("initialDisplayState returned error: %v", err)
	}
	layout := mustStoreLayout(t, workspace)
	assertDir(t, layout.StoreRoot)
	assertDir(t, layout.ArtifactsRoot)
	assertDir(t, layout.IndexesRoot)
	assertFileContent(t, layout.ProjectFile, "schema_version = 1\n")
	assertStoreEntries(t, layout.StoreRoot, []string{"artifacts/", "indexes/", "project.toml"})

	render := tui.RenderPlain(view, tui.Size{Width: 120, Height: 32})
	for _, token := range []string{"project store: initialized", "Model opencode-go/deepseek-v4-pro:high"} {
		if !strings.Contains(render, token) {
			t.Fatalf("startup render missing store token %q:\n%s", token, render)
		}
	}
	if strings.Contains(render, workspace) {
		t.Fatalf("startup render leaked workspace path %q:\n%s", workspace, render)
	}

	semantic := tui.Semantic(view, tui.Size{Width: 120, Height: 32})
	if semantic.Session.ProjectStoreStatus != "initialized" || semantic.Session.ProjectStoreSource != "state.open" || semantic.Session.ProjectStoreDetail != "project store ready" {
		t.Fatalf("semantic store status = %+v, want initialized state.open ready", semantic.Session)
	}
	if !semanticRegionContains(semantic, "project_store", []string{"status: initialized", "source: state.open", "detail: project store ready", "app-owned"}) {
		t.Fatalf("semantic project_store region missing initialized state: %+v", semantic.Regions)
	}
}

func TestInitialDisplayStateStoreFailureIsPathSafeDegradedStatus(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	layout := mustStoreLayout(t, workspace)
	if err := os.MkdirAll(layout.ProjectFile, 0o755); err != nil {
		t.Fatalf("create project metadata directory: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	view, err := initialDisplayState(context.Background(), workspace)
	if err != nil {
		t.Fatalf("initialDisplayState returned error: %v", err)
	}
	if view.ProjectStoreStatus != "degraded" || view.ProjectStoreSource != "state.open" {
		t.Fatalf("store display status = %+v, want degraded state.open", view)
	}
	for _, value := range []string{view.ProjectStoreDetail, tui.RenderPlain(view, tui.Size{Width: 120, Height: 32})} {
		if strings.Contains(value, workspace) || strings.Contains(value, layout.ProjectFile) {
			t.Fatalf("degraded store status leaked path: %q", value)
		}
	}
	if !strings.Contains(view.ProjectStoreDetail, "project metadata path is a directory") {
		t.Fatalf("degraded detail = %q, want bounded failure reason", view.ProjectStoreDetail)
	}
}

func TestInitialDisplayStateReopenPreservesProjectStoreMetadata(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	layout := mustStoreLayout(t, workspace)
	metadata := "schema_version = 1\nproject_id = \"preserve\"\n"
	writeFile(t, layout.ProjectFile, metadata)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	if _, err := initialDisplayState(context.Background(), workspace); err != nil {
		t.Fatalf("first initialDisplayState returned error: %v", err)
	}
	if _, err := initialDisplayState(context.Background(), workspace); err != nil {
		t.Fatalf("second initialDisplayState returned error: %v", err)
	}

	assertFileContent(t, layout.ProjectFile, metadata)
	assertStoreEntries(t, layout.StoreRoot, []string{"artifacts/", "indexes/", "project.toml"})
}

func TestStoreDisplayStatusRejectsRawStateErrorPaths(t *testing.T) {
	for _, err := range []error{
		errors.New("open /tmp/sensitive-workspace/.aila/project.toml: permission denied"),
		errors.New("create store directory /tmp/sensitive-workspace/.aila: permission denied"),
		errors.New("replace artifact file: rename /tmp/sensitive-workspace/.aila/artifacts/.project-summary.md.tmp /tmp/sensitive-workspace/.aila/artifacts/project-summary.md: permission denied"),
	} {
		detail := boundedStoreError(err)
		if strings.Contains(detail, "/tmp") || strings.Contains(detail, ".aila") {
			t.Fatalf("bounded store error leaked path for %v: %q", err, detail)
		}
	}
}

func mustStoreLayout(t *testing.T, workspace string) state.Layout {
	t.Helper()
	layout, err := state.DescribeStore(workspace)
	if err != nil {
		t.Fatalf("DescribeStore(%q): %v", workspace, err)
	}
	return layout
}

func assertDir(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat directory %q: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", path)
	}
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("content of %q = %q, want %q", path, content, want)
	}
}

func assertStoreEntries(t *testing.T, root string, want []string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read store root %q: %v", root, err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		got = append(got, name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("store entries = %v, want %v", got, want)
	}
}

func semanticRegionContains(snapshot tui.SemanticSnapshot, name string, tokens []string) bool {
	for _, region := range snapshot.Regions {
		if region.Name != name {
			continue
		}
		joined := strings.Join(region.Items, "\n")
		for _, token := range tokens {
			if !strings.Contains(joined, token) {
				return false
			}
		}
		return true
	}
	return false
}

package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/diagnostic"
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
	if len(view.Diagnostics) != 1 {
		t.Fatalf("diagnostics length = %d, want 1", len(view.Diagnostics))
	}
	got := view.Diagnostics[0]
	if got.Severity != "error" || got.Source != "state.open" || got.AffectedArtifact != "project_store" || got.RecoveryAction != "inspect" || !got.UserInputNeeded {
		t.Fatalf("degraded diagnostic fields = %+v", got)
	}
	if !strings.Contains(got.BoundedMessage, "project store unavailable") || strings.Contains(got.BoundedMessage, workspace) || strings.Contains(got.BoundedMessage, layout.ProjectFile) {
		t.Fatalf("degraded diagnostic message = %q, want bounded path-safe unavailable state", got.BoundedMessage)
	}
}

func TestInitialDisplayStateSurfacesRecoveryNeededStoreStatus(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	layout := mustStoreLayout(t, workspace)
	metadata := "schema_version = 1\nproject_id = [unterminated\n"
	writeFile(t, layout.ProjectFile, metadata)
	writeFile(t, filepath.Join(layout.ArtifactsRoot, "keep.txt"), "artifact\n")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	view, err := initialDisplayState(context.Background(), workspace)
	if err != nil {
		t.Fatalf("initialDisplayState returned error: %v", err)
	}

	if view.ProjectStoreStatus != "recovery_needed" || view.ProjectStoreSource != "state.open" {
		t.Fatalf("store display status = %+v, want recovery_needed state.open", view)
	}
	for _, value := range []string{view.ProjectStoreDetail, tui.RenderPlain(view, tui.Size{Width: 120, Height: 32})} {
		if strings.Contains(value, workspace) || strings.Contains(value, layout.ProjectFile) {
			t.Fatalf("recovery store status leaked path: %q", value)
		}
	}
	if !strings.Contains(view.ProjectStoreDetail, "project metadata requires recovery") {
		t.Fatalf("recovery detail = %q, want recovery reason", view.ProjectStoreDetail)
	}
	for _, token := range []string{"error", "state.open", "project_metadata", "manual_repair", "user_input_needed=true"} {
		if !strings.Contains(view.ProjectStoreDetail, token) {
			t.Fatalf("recovery detail = %q, want actionable token %q", view.ProjectStoreDetail, token)
		}
	}
	if len(view.Diagnostics) != 1 {
		t.Fatalf("diagnostics length = %d, want 1", len(view.Diagnostics))
	}
	semantic := tui.Semantic(view, tui.Size{Width: 120, Height: 32})
	if len(semantic.Diagnostics) != 1 {
		t.Fatalf("semantic diagnostics length = %d, want 1", len(semantic.Diagnostics))
	}
	got := semantic.Diagnostics[0]
	if got.Severity != "error" || got.Source != "state.open" || got.AffectedArtifact != "project_metadata" || got.RecoveryAction != "manual_repair" || !got.UserInputNeeded {
		t.Fatalf("semantic diagnostic fields = %+v", got)
	}
	assertFileContent(t, layout.ProjectFile, metadata)
	assertFileContent(t, filepath.Join(layout.ArtifactsRoot, "keep.txt"), "artifact\n")
}

func TestStartupDebugDiagnosticsOutputIsStructuredBoundedAndRedacted(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	layout := mustStoreLayout(t, workspace)
	writeFile(t, layout.ProjectFile, "schema_version = 2\nsecret = \"token=abc123\"\n")

	debugOutput := NewDebugDiagnosticsOutput(startupDiagnostics(context.Background(), workspace))
	encoded, err := debugOutput.JSON()
	if err != nil {
		t.Fatalf("debug diagnostics JSON: %v", err)
	}
	if len(encoded) > MaxDebugDiagnosticOutputBytes {
		t.Fatalf("debug diagnostics output length = %d, want <= %d", len(encoded), MaxDebugDiagnosticOutputBytes)
	}
	for _, leaked := range []string{workspace, layout.ProjectFile, ".aila", "abc123", "token="} {
		if strings.Contains(encoded, leaked) {
			t.Fatalf("debug diagnostics leaked %q in %s", leaked, encoded)
		}
	}

	var decoded DebugDiagnosticsOutput
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("unmarshal debug diagnostics: %v", err)
	}
	if decoded.Count != 1 || decoded.MaxCount != MaxDebugDiagnostics || decoded.MaxMessageBytes != 240 || decoded.MaxOutputBytes != MaxDebugDiagnosticOutputBytes {
		t.Fatalf("debug diagnostics bounds = %+v", decoded)
	}
	got := decoded.Diagnostics[0]
	if got.Severity != "error" || got.Source != "state.open" || got.AffectedArtifact != "project_metadata" || got.RecoveryAction != "manual_repair" || !got.UserInputNeeded || got.BoundedMessage == "" {
		t.Fatalf("debug diagnostic fields = %+v", got)
	}
}

func TestDebugDiagnosticsOutputRedactsCommonCredentialForms(t *testing.T) {
	debugOutput := NewDebugDiagnosticsOutput([]diagnostic.Diagnostic{diagnostic.New(diagnostic.Spec{
		Category: diagnostic.CategoryStartup,
		Source:   diagnostic.SourceStartup,
		Severity: diagnostic.SeverityError,
		Message: strings.Join([]string{
			"Authorization: Bearer sk-live-secret",
			"password: hunter2",
			"token abc123",
			"apikey=plain-key",
			"apiKey=camel-key",
			"api_key=snake-key",
			"https://user:pass@example.com/path",
		}, " "),
		AffectedArtifact: diagnostic.ArtifactProviderRequest,
		RecoveryAction:   diagnostic.RecoveryInspect,
	})})
	encoded, err := debugOutput.JSON()
	if err != nil {
		t.Fatalf("debug diagnostics JSON: %v", err)
	}

	for _, leaked := range []string{
		"Authorization", "Bearer", "sk-live-secret",
		"password", "hunter2",
		"token", "abc123",
		"apikey", "apiKey", "api_key", "plain-key", "camel-key", "snake-key",
		"user:pass", "pass@example.com",
	} {
		if strings.Contains(encoded, leaked) {
			t.Fatalf("debug diagnostics leaked %q in %s", leaked, encoded)
		}
	}
}

func TestStartupDebugDiagnosticsOutputReportsEmptySet(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")

	debugOutput := NewDebugDiagnosticsOutput(startupDiagnostics(context.Background(), workspace))
	encoded, err := debugOutput.JSON()
	if err != nil {
		t.Fatalf("debug diagnostics JSON: %v", err)
	}

	var decoded DebugDiagnosticsOutput
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("unmarshal debug diagnostics: %v", err)
	}
	if decoded.Count != 0 || len(decoded.Diagnostics) != 0 {
		t.Fatalf("debug diagnostics = %+v, want empty set", decoded)
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

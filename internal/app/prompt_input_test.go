package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

func TestPromptEditorCommandAppliesEditedPromptThroughAppHandler(t *testing.T) {
	t.Parallel()

	state := tui.ApplyPromptInputText(tui.IdleEmptyState(), "draft prompt")
	controller := newSessionControllerWithPersistence(context.Background(), state, newInputRunnerWithDispatch(noRuntimeDispatch), nil)
	controller.promptEditor = func(ctx context.Context, request promptEditorRequest) promptEditorResult {
		if request.InitialText != "draft prompt" {
			t.Fatalf("editor initial text = %q, want draft prompt", request.InitialText)
		}
		return promptEditorResult{Status: "applied", Text: "edited\nline two\nline three", Detail: "test editor applied"}
	}

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteEditor, Kind: policy.CommandInputShortcut}, controller.view)
	if got.PromptInput != "edited\nline two\nline three" {
		t.Fatalf("prompt input = %q, want edited text", got.PromptInput)
	}
	if got.PromptEditor == nil || got.PromptEditor.Status != "applied" || got.SurfaceTitle != "editor" {
		t.Fatalf("editor view = %+v surface=%q", got.PromptEditor, got.SurfaceTitle)
	}
	if got.PromptPaste == nil || got.PromptDisplayInput != "[Pasted lines +3]" {
		t.Fatalf("prompt display metadata = display %q paste %+v", got.PromptDisplayInput, got.PromptPaste)
	}
}

func TestPromptEditorCommandRestoresPromptOnCancelAndFailure(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		result promptEditorResult
		status string
	}{
		{name: "cancel", result: promptEditorResult{Status: "canceled", Detail: "closed unchanged"}, status: "canceled"},
		{name: "failure", result: promptEditorResult{Status: "failed", Detail: "editor failed"}, status: "failed"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state := tui.ApplyPromptInputText(tui.IdleEmptyState(), "unchanged draft")
			controller := newSessionControllerWithPersistence(context.Background(), state, newInputRunnerWithDispatch(noRuntimeDispatch), nil)
			controller.promptEditor = func(context.Context, promptEditorRequest) promptEditorResult { return tc.result }

			got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteEditor, Kind: policy.CommandInputSlash}, controller.view)
			if got.PromptInput != "unchanged draft" {
				t.Fatalf("prompt input after %s = %q, want unchanged draft", tc.name, got.PromptInput)
			}
			if got.PromptEditor == nil || got.PromptEditor.Status != tc.status {
				t.Fatalf("editor status after %s = %+v", tc.name, got.PromptEditor)
			}
		})
	}
}

func TestFileReferenceDiscoveryUsesReadOnlyAppFindBoundary(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	for _, path := range []string{"README.md", "docs/guide.md", "internal/app.go", ".aila/hidden.md", ".git/config"} {
		full := filepath.Join(workspace, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("create fixture dir for %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte("fixture"), 0o644); err != nil {
			t.Fatalf("write fixture file %s: %v", path, err)
		}
	}

	controller := newSessionControllerWithPersistence(context.Background(), tui.IdleEmptyState(), newInputRunnerWithDispatch(noRuntimeDispatch), nil)
	controller.workspacePath = workspace
	controller.autonomyLevel = "read"
	got := controller.discoverPromptFileReferences("guide", tui.ApplyPromptInputText(controller.view, "see @"))

	if got.FileReference == nil || !got.FileReference.Focus || got.FileReference.Status != "ready" {
		t.Fatalf("file-reference view = %+v", got.FileReference)
	}
	if len(got.FileReference.Items) != 1 || got.FileReference.Items[0].Path != "docs/guide.md" {
		t.Fatalf("file-reference rows = %+v, want docs/guide.md only", got.FileReference.Items)
	}
	joined := strings.Join(got.SurfaceLines, "\n")
	for _, forbidden := range []string{".aila", ".git", workspace} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("file-reference surface leaked forbidden marker %q: %s", forbidden, joined)
		}
	}
}

func noRuntimeDispatch([]runtime.Effect) []runtime.Message { return nil }

func TestFileReferenceDiscoveryUsesDynamicPattern(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	for _, path := range []string{"docs/guide.md", "docs/other.md"} {
		full := filepath.Join(workspace, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("create fixture dir: %v", err)
		}
		if err := os.WriteFile(full, []byte("fixture"), 0o644); err != nil {
			t.Fatalf("write fixture file: %v", err)
		}
	}

	controller := newSessionControllerWithPersistence(context.Background(), tui.IdleEmptyState(), newInputRunnerWithDispatch(noRuntimeDispatch), nil)
	controller.workspacePath = workspace
	controller.autonomyLevel = "read"

	// Verify that querying "guide" matches only "docs/guide.md"
	got := controller.discoverPromptFileReferences("guide", controller.view)
	if got.FileReference == nil || got.FileReference.Status != "ready" {
		t.Fatalf("file reference not ready: %+v", got.FileReference)
	}
	if len(got.FileReference.Items) != 1 || got.FileReference.Items[0].Path != "docs/guide.md" {
		t.Fatalf("expected only docs/guide.md, got: %+v", got.FileReference.Items)
	}

	// Verify that querying "other" matches only "docs/other.md"
	got = controller.discoverPromptFileReferences("other", controller.view)
	if got.FileReference == nil || got.FileReference.Status != "ready" {
		t.Fatalf("file reference not ready: %+v", got.FileReference)
	}
	if len(got.FileReference.Items) != 1 || got.FileReference.Items[0].Path != "docs/other.md" {
		t.Fatalf("expected only docs/other.md, got: %+v", got.FileReference.Items)
	}
}

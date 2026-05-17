package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

type promptEditorRequest struct {
	InitialText string
}

type promptEditorResult struct {
	Status string
	Text   string
	Detail string
}

type promptEditorRunner func(context.Context, promptEditorRequest) promptEditorResult

func (controller *sessionController) openPromptEditorView() {
	initial := controller.view.PromptInput
	runner := controller.promptEditor
	if runner == nil {
		runner = runExternalPromptEditor
	}
	result := runner(controller.ctx, promptEditorRequest{InitialText: initial})
	switch result.Status {
	case "applied":
		controller.view = tui.ApplyPromptInputText(controller.view, result.Text)
		controller.view = tui.ApplyPromptEditorView(controller.view, &tui.PromptEditorView{Source: "app.editor", Status: "applied", Detail: defaultString(result.Detail, "edited prompt applied")})
	case "canceled":
		controller.view = tui.ApplyPromptInputText(controller.view, initial)
		controller.view = tui.ApplyPromptEditorView(controller.view, &tui.PromptEditorView{Source: "app.editor", Status: "canceled", Detail: defaultString(result.Detail, "prompt unchanged")})
	default:
		controller.view = tui.ApplyPromptInputText(controller.view, initial)
		detail := result.Detail
		if detail == "" {
			detail = "editor command failed"
		}
		controller.view = tui.ApplyPromptEditorView(controller.view, &tui.PromptEditorView{Source: "app.editor", Status: "failed", Detail: detail})
	}
}

func runExternalPromptEditor(ctx context.Context, request promptEditorRequest) promptEditorResult {
	editor := firstNonEmpty(os.Getenv("VISUAL"), os.Getenv("EDITOR"))
	if editor == "" {
		return promptEditorResult{Status: "failed", Detail: "set VISUAL or EDITOR to use /editor"}
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return promptEditorResult{Status: "failed", Detail: "editor command is empty"}
	}

	file, err := os.CreateTemp("", "aila-prompt-*.md")
	if err != nil {
		return promptEditorResult{Status: "failed", Detail: "create editor buffer failed"}
	}
	name := file.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := file.WriteString(request.InitialText); err != nil {
		_ = file.Close()
		return promptEditorResult{Status: "failed", Detail: "write editor buffer failed"}
	}
	if err := file.Close(); err != nil {
		return promptEditorResult{Status: "failed", Detail: "close editor buffer failed"}
	}

	args := append([]string{}, parts[1:]...)
	args = append(args, name)
	cmd := exec.CommandContext(ctx, parts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return promptEditorResult{Status: "failed", Detail: "editor exited without applying changes"}
	}
	edited, err := os.ReadFile(name)
	if err != nil {
		return promptEditorResult{Status: "failed", Detail: "read editor buffer failed"}
	}
	text := string(edited)
	if text == request.InitialText {
		return promptEditorResult{Status: "canceled", Text: request.InitialText, Detail: "editor closed without changes"}
	}
	return promptEditorResult{Status: "applied", Text: text, Detail: "edited prompt applied"}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (controller *sessionController) discoverPromptFileReferences(query string, view tui.ViewState) tui.ViewState {
	controller.view = view
	controller.view = tui.ApplyFileReferenceView(controller.view, controller.promptFileReferenceView(query))
	return controller.view
}

func (controller *sessionController) promptFileReferenceView(query string) *tui.FileReferenceView {
	if controller.workspacePath == "" {
		return &tui.FileReferenceView{Source: "app.file-reference", Status: "failed", Query: query, Detail: "workspace unavailable", Focus: true}
	}
	message := dispatchSearchEffect(controller.ctx, controller.workspacePath, permission.AutonomyLevel(controller.autonomyLevel), runtime.SearchToolEffect{
		Operation: runtime.OperationMetadata{ID: "prompt-file-reference", Kind: runtime.OperationFind, Subject: "**/*", Source: "prompt"},
		Request: runtime.SearchToolRequest{
			ToolName:        runtime.SearchToolFind,
			Pattern:         "**/*",
			MaxResults:      50,
			MaxPreviewBytes: 120,
			Source: runtime.SearchSourceMetadata{
				Caller:      "app.prompt.file-reference",
				RequestID:   "prompt-file-reference",
				Description: "discover workspace files for prompt @ references",
			},
		},
	})
	completed, ok := message.(runtime.SearchToolCompleted)
	if !ok {
		return &tui.FileReferenceView{Source: "app.file-reference", Status: "failed", Query: query, Detail: "file discovery did not complete", Focus: true}
	}
	if completed.Result.Error.Kind != "" && completed.Result.Error.Kind != runtime.SearchToolErrorNone {
		return &tui.FileReferenceView{Source: "app.file-reference", Status: "failed", Query: query, Detail: completed.Result.Error.Message, Focus: true}
	}

	items := promptFileReferenceItems(query, completed.Result.Matches)
	status := "ready"
	detail := fmt.Sprintf("%d files", len(items))
	if len(items) == 0 {
		status = "empty"
		detail = "no matching files"
	}
	return &tui.FileReferenceView{Source: "app.file-reference", Status: status, Query: query, Detail: detail, Items: items, Focus: true}
}

func promptFileReferenceItems(query string, matches []runtime.SearchToolMatch) []tui.FileReferenceItemView {
	query = strings.ToLower(strings.TrimSpace(query))
	items := make([]tui.FileReferenceItemView, 0, len(matches))
	for _, match := range matches {
		path := strings.TrimSpace(match.Path)
		if path == "" {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(path), query) {
			continue
		}
		items = append(items, tui.FileReferenceItemView{Path: path, Detail: "read-only discovery"})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	if len(items) > 10 {
		items = items[:10]
	}
	return items
}

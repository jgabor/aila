package app

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/jgabor/aila/internal/tools"
	"github.com/jgabor/aila/internal/tui"
)

// DiffReadCommand is an explicit app-owned request to inspect current changes.
type DiffReadCommand struct{}

// DiffReadResult is the typed outcome of opening the read-only diff view.
type DiffReadResult struct {
	View *tui.DiffView
}

type diffReadFunc func(context.Context, DiffReadCommand) DiffReadResult

var unifiedHunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func storeCurrentDiffRead(workspacePath string) diffReadFunc {
	return func(ctx context.Context, _ DiffReadCommand) DiffReadResult {
		text, failure := readCurrentUnifiedDiff(ctx, workspacePath)
		if failure != nil {
			return DiffReadResult{View: failure}
		}
		return DiffReadResult{View: parseUnifiedDiffView("git diff", text)}
	}
}

func readCurrentUnifiedDiff(ctx context.Context, workspacePath string) (string, *tui.DiffView) {
	cached, cachedFailure := runCurrentDiffCommand(ctx, workspacePath, "current-workspace-staged-diff", []string{"git", "diff", "--cached", "--color=never", "-U0"})
	worktree, worktreeFailure := runCurrentDiffCommand(ctx, workspacePath, "current-workspace-unstaged-diff", []string{"git", "diff", "--color=never", "-U0"})
	if cachedFailure != nil && worktreeFailure != nil {
		return "", cachedFailure
	}
	if cachedFailure != nil {
		return worktree, nil
	}
	if worktreeFailure != nil {
		return cached, nil
	}
	return joinDiffOutputs(cached, worktree), nil
}

func runCurrentDiffCommand(ctx context.Context, workspacePath string, requestID string, argv []string) (string, *tui.DiffView) {
	validated, bashErr := tools.ValidateBashRequest(workspacePath, tools.BashRequest{
		Argv:           argv,
		WorkingDir:     ".",
		MaxOutputBytes: 96 * 1024,
		TimeoutMillis:  3000,
		Source: tools.BashSourceMetadata{
			Caller:      "diff-view",
			RequestID:   requestID,
			Description: "inspect current workspace diff for display",
		},
	})
	if bashErr.Kind != "" {
		return "", failedDiffView("git diff", bashErr.Message)
	}
	result := tools.ExecuteBash(ctx, validated)
	if result.Error.Kind != "" && result.Error.Kind != tools.BashErrorNone {
		return "", failedDiffView("git diff", result.Error.Message)
	}
	return result.Stdout.Text, nil
}

func joinDiffOutputs(parts ...string) string {
	joined := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimRight(part, "\n")
		if strings.TrimSpace(part) != "" {
			joined = append(joined, part)
		}
	}
	return strings.Join(joined, "\n")
}

func failedDiffView(source string, message string) *tui.DiffView {
	return &tui.DiffView{Source: source, Status: "failed", Empty: true, ErrorMessage: boundedDiffMessage(message)}
}

func emptyDiffView(source string) *tui.DiffView {
	return &tui.DiffView{Source: source, Status: "empty", Empty: true}
}

func parseUnifiedDiffView(source string, text string) *tui.DiffView {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return emptyDiffView(source)
	}
	view := &tui.DiffView{Source: source, Status: "ready"}
	var file *tui.DiffFileView
	var hunk *tui.DiffHunkView
	oldLine := 0
	newLine := 0
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			view.Files = append(view.Files, tui.DiffFileView{Status: "modified"})
			file = &view.Files[len(view.Files)-1]
			hunk = nil
		case file != nil && strings.HasPrefix(line, "new file mode "):
			file.Status = "added"
		case file != nil && strings.HasPrefix(line, "deleted file mode "):
			file.Status = "deleted"
		case file != nil && strings.HasPrefix(line, "--- "):
			file.OldPath = normalizeDiffPath(strings.TrimSpace(strings.TrimPrefix(line, "--- ")))
			if file.OldPath == "/dev/null" && file.Status == "modified" {
				file.Status = "added"
			}
		case file != nil && strings.HasPrefix(line, "+++ "):
			file.Path = normalizeDiffPath(strings.TrimSpace(strings.TrimPrefix(line, "+++ ")))
			if file.Path == "/dev/null" {
				file.Path = file.OldPath
				file.Status = "deleted"
			}
		case file != nil && strings.HasPrefix(line, "@@ "):
			oldStart, oldCount, newStart, newCount := parseUnifiedHunkHeader(line)
			file.Hunks = append(file.Hunks, tui.DiffHunkView{Header: line, OldStart: oldStart, OldLines: oldCount, NewStart: newStart, NewLines: newCount})
			hunk = &file.Hunks[len(file.Hunks)-1]
			oldLine = oldStart
			newLine = newStart
		case hunk != nil:
			if line == `\ No newline at end of file` {
				continue
			}
			if strings.HasPrefix(line, "+") {
				hunk.Lines = append(hunk.Lines, tui.DiffLineView{Kind: "addition", Text: strings.TrimPrefix(line, "+"), NewLine: newLine})
				newLine++
				continue
			}
			if strings.HasPrefix(line, "-") {
				hunk.Lines = append(hunk.Lines, tui.DiffLineView{Kind: "removal", Text: strings.TrimPrefix(line, "-"), OldLine: oldLine})
				oldLine++
				continue
			}
			if strings.HasPrefix(line, " ") {
				hunk.Lines = append(hunk.Lines, tui.DiffLineView{Kind: "context", Text: strings.TrimPrefix(line, " "), OldLine: oldLine, NewLine: newLine})
				oldLine++
				newLine++
			}
		}
	}
	for index := range view.Files {
		if view.Files[index].Path == "" {
			view.Files[index].Path = view.Files[index].OldPath
		}
		if view.Files[index].Status == "" {
			view.Files[index].Status = "modified"
		}
	}
	if len(view.Files) == 0 {
		return emptyDiffView(source)
	}
	return view
}

func normalizeDiffPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	if path == "" {
		return "unknown"
	}
	return path
}

func parseUnifiedHunkHeader(header string) (int, int, int, int) {
	match := unifiedHunkHeader.FindStringSubmatch(header)
	if len(match) == 0 {
		return 0, 0, 0, 0
	}
	oldStart := atoiDefault(match[1], 0)
	oldCount := atoiDefault(match[2], 1)
	newStart := atoiDefault(match[3], 0)
	newCount := atoiDefault(match[4], 1)
	return oldStart, oldCount, newStart, newCount
}

func atoiDefault(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func boundedDiffMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "diff unavailable"
	}
	if len(message) > 160 {
		return message[:160] + "..."
	}
	return message
}

func (controller *sessionController) openDiffView() {
	if controller.readDiff == nil {
		controller.view = tui.ApplyDiffView(controller.view, emptyDiffView("app.diff"), 0, true)
		return
	}
	result := controller.readDiff(controller.ctx, DiffReadCommand{})
	controller.view = tui.ApplyDiffView(controller.view, result.View, 0, true)
}

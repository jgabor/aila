package tui

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/policy"
)

// PromptEditorView is app-injected editor command state. It is display-only;
// TUI code must never launch editors or read temporary files itself.
type PromptEditorView struct {
	Source string
	Status string
	Detail string
}

// FileReferenceView is app-injected project file discovery state. It is
// display-only; TUI code must never walk directories or read files itself.
type FileReferenceView struct {
	Source   string
	Status   string
	Query    string
	Detail   string
	Items    []FileReferenceItemView
	Selected int
	Focus    bool
}

// FileReferenceItemView records one app-discovered relative file path.
type FileReferenceItemView struct {
	Path   string
	Detail string
}

// PromptPasteView records exact/display prompt metadata for summarized pasted text.
type PromptPasteView struct {
	Summary            string
	LineCount          int
	ByteCount          int
	ExactTextRef       string
	ExactTextSHA256    string
	PreservedExactText bool
}

// ApplyPromptInputText replaces exact prompt text and refreshes display metadata.
func ApplyPromptInputText(state ViewState, text string) ViewState {
	state.PromptInput = text
	refreshPromptDisplay(&state)
	return state
}

func appendPromptInputText(state ViewState, text string) ViewState {
	state.PromptInput += text
	refreshPromptDisplay(&state)
	return state
}

func clearPromptInput(state ViewState) ViewState {
	return ApplyPromptInputText(state, "")
}

func dropPromptInputRune(state ViewState) ViewState {
	state.PromptInput = dropLastRune(state.PromptInput)
	refreshPromptDisplay(&state)
	return state
}

func refreshPromptDisplay(state *ViewState) {
	state.PromptDisplayInput = state.PromptInput
	state.PromptPaste = nil
	lineCount := promptLineCount(state.PromptInput)
	if lineCount <= 2 {
		return
	}
	summary := fmt.Sprintf("[Pasted lines +%d]", lineCount)
	state.PromptDisplayInput = summary
	state.PromptPaste = &PromptPasteView{
		Summary:            summary,
		LineCount:          lineCount,
		ByteCount:          len(state.PromptInput),
		ExactTextRef:       "prompt_input",
		ExactTextSHA256:    promptExactHash(state.PromptInput),
		PreservedExactText: true,
	}
}

func promptDisplayText(state ViewState) string {
	display := state.PromptDisplayInput
	if display == "" && state.PromptInput != "" {
		display = state.PromptInput
	}
	return escapePromptLineBreaks(display)
}

func escapePromptLineBreaks(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.ReplaceAll(value, "\n", `\n`)
}

func promptLineCount(value string) int {
	if value == "" {
		return 0
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.Count(value, "\n") + 1
}

func promptExactHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("sha256:%x", sum)
}

// ApplyPromptEditorView injects app-owned prompt editor status into visible state.
func ApplyPromptEditorView(state ViewState, editor *PromptEditorView) ViewState {
	if editor == nil {
		return state
	}
	state.CommandRoute = string(policy.CommandRouteEditor)
	state.RouteSource = "policy.command"
	state.SurfaceTitle = "editor"
	state.PromptEditor = clonePromptEditorView(editor)
	state.FileReference = nil
	state.SurfaceLines = promptEditorSurfaceLines(*state.PromptEditor)
	return state
}

func clonePromptEditorView(editor *PromptEditorView) *PromptEditorView {
	if editor == nil {
		return nil
	}
	clone := *editor
	return &clone
}

func promptEditorSurfaceLines(editor PromptEditorView) []string {
	lines := []string{
		"source: " + safeText(defaultString(editor.Source, "app.editor")),
		"status: " + safeText(editor.Status),
	}
	if editor.Detail != "" {
		lines = append(lines, "detail: "+safeText(editor.Detail))
	}
	return append(lines, "app-owned", "display-only")
}

// ApplyFileReferenceView injects app-owned file-reference discovery rows.
func ApplyFileReferenceView(state ViewState, refs *FileReferenceView) ViewState {
	if refs == nil {
		return state
	}
	state.RouteSource = defaultString(refs.Source, "app.file-reference")
	state.SurfaceTitle = "file-reference"
	state.FileReference = cloneFileReferenceView(refs)
	state.FileReference.Selected = clampFileReferenceSelection(*state.FileReference)
	state.SurfaceLines = fileReferenceSurfaceLines(*state.FileReference)
	return state
}

func cloneFileReferenceView(refs *FileReferenceView) *FileReferenceView {
	if refs == nil {
		return nil
	}
	clone := *refs
	clone.Items = append([]FileReferenceItemView(nil), refs.Items...)
	return &clone
}

func clampFileReferenceSelection(refs FileReferenceView) int {
	if len(refs.Items) == 0 {
		return 0
	}
	if refs.Selected < 0 {
		return 0
	}
	if refs.Selected >= len(refs.Items) {
		return len(refs.Items) - 1
	}
	return refs.Selected
}

func fileReferenceSurfaceLines(refs FileReferenceView) []string {
	selected := clampFileReferenceSelection(refs)
	lines := []string{
		"source: " + safeText(defaultString(refs.Source, "app.file-reference")),
		"status: " + safeText(defaultString(refs.Status, "ready")),
		"query: " + safeText(refs.Query),
		fmt.Sprintf("selected: %d", selected+1),
	}
	if refs.Detail != "" {
		lines = append(lines, "detail: "+safeText(refs.Detail))
	}
	if len(refs.Items) > 0 {
		lines = append(lines, "files:")
		for index, item := range refs.Items {
			marker := " "
			if index == selected {
				marker = ">"
			}
			line := marker + " " + safeReadTargetPath(item.Path)
			if item.Detail != "" {
				line += " detail=" + safeText(item.Detail)
			}
			lines = append(lines, line)
		}
	}
	if refs.Focus {
		lines = append(lines, "focus: file-reference")
	}
	return append(lines, "read-only", "app-owned", "display-only")
}

func semanticPromptItems(state ViewState) []string {
	display := promptDisplayText(state)
	items := []string{promptLine(display)}
	if display != escapePromptLineBreaks(state.PromptInput) || state.PromptPaste != nil {
		items = append(items,
			"display_text: "+safeText(display),
			"exact_text_ref: prompt_input",
			fmt.Sprintf("exact_text_lines: %d", promptLineCount(state.PromptInput)),
			"exact_text_sha256: "+promptExactHash(state.PromptInput),
			"exact_text_preserved: true",
		)
	}
	if state.PromptPaste != nil {
		items = append(items,
			"paste_summary: "+state.PromptPaste.Summary,
			fmt.Sprintf("paste_line_count: %d", state.PromptPaste.LineCount),
			fmt.Sprintf("paste_byte_count: %d", state.PromptPaste.ByteCount),
		)
	}
	for _, ref := range promptFileReferenceLinks(state.PromptInput) {
		items = append(items, "file_ref: "+safeReadTargetPath(ref))
	}
	return items
}

func semanticFileReferenceItems(refs *FileReferenceView) []string {
	if refs == nil {
		return nil
	}
	selected := clampFileReferenceSelection(*refs)
	items := []string{
		"source: " + safeText(defaultString(refs.Source, "app.file-reference")),
		"status: " + safeText(defaultString(refs.Status, "ready")),
		"query: " + safeText(refs.Query),
		"focus: " + boolLabel(refs.Focus),
		fmt.Sprintf("selected_index: %d", selected),
		"read_only: true",
		"app_owned: true",
	}
	if refs.Detail != "" {
		items = append(items, "detail: "+safeText(refs.Detail))
	}
	for index, item := range refs.Items {
		items = append(items, "item: "+safeReadTargetPath(item.Path)+" selected="+boolLabel(index == selected))
	}
	return append(items, "display-only")
}

func fileReferenceActions(refs *FileReferenceView) []SemanticAction {
	if refs == nil || !refs.Focus {
		return nil
	}
	return []SemanticAction{
		{Name: "insert file reference", Input: "enter", Default: true, PresentationOnly: true, Executed: false},
		{Name: "close file reference picker", Input: "esc", PresentationOnly: true, Executed: false},
		{Name: "move file reference selection", Input: "up/down", PresentationOnly: true, Executed: false},
	}
}

func promptFileReferenceLinks(input string) []string {
	fields := strings.Fields(input)
	refs := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, " ,.;:()[]{}<>")
		if strings.HasPrefix(field, "@") && len(field) > 1 {
			refs = append(refs, strings.TrimPrefix(field, "@"))
		}
	}
	return refs
}

func insertSelectedFileReference(state ViewState) ViewState {
	if state.FileReference == nil || len(state.FileReference.Items) == 0 {
		return closeFileReference(state, "empty", "no file reference selected")
	}
	refs := cloneFileReferenceView(state.FileReference)
	selected := clampFileReferenceSelection(*refs)
	link := "@" + refs.Items[selected].Path
	state = ApplyPromptInputText(state, replaceActiveReferenceToken(state.PromptInput, link))
	refs.Focus = false
	refs.Status = "inserted"
	refs.Detail = "inserted " + refs.Items[selected].Path
	refs.Selected = selected
	return ApplyFileReferenceView(state, refs)
}

func closeFileReference(state ViewState, status string, detail string) ViewState {
	if state.FileReference == nil {
		return state
	}
	refs := cloneFileReferenceView(state.FileReference)
	refs.Focus = false
	refs.Status = status
	refs.Detail = detail
	return ApplyFileReferenceView(state, refs)
}

func replaceActiveReferenceToken(input string, link string) string {
	at := strings.LastIndex(input, "@")
	if at < 0 {
		if strings.TrimSpace(input) == "" {
			return link
		}
		return input + " " + link
	}
	end := at + 1
	for end < len(input) {
		r := rune(input[end])
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			break
		}
		end++
	}
	return input[:at] + link + input[end:]
}

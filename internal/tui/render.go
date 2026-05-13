package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiCyan   = "\x1b[36m"
	ansiYellow = "\x1b[33m"
	ansiReset  = "\x1b[0m"
)

// ViewState is the deterministic data rendered by the M2 static shell.
type ViewState struct {
	Scenario      string
	AppName       string
	Phase         string
	PhaseSource   string
	PrimaryModel  string
	UtilityModel  string
	Autonomy      string
	FooterGit     string
	FooterContext string
	Transcript    []TranscriptTurn
	CommandRoute  string
	RouteSource   string
	SurfaceTitle  string
	SurfaceLines  []string
	PromptInput   string
}

// IdleEmptyState returns the static first-launch view state.
func IdleEmptyState() ViewState {
	return ViewState{
		Scenario:      "idle-empty",
		AppName:       "Aila",
		Phase:         "placeholder",
		PhaseSource:   "not_started",
		PrimaryModel:  "placeholder",
		UtilityModel:  "placeholder",
		Autonomy:      "placeholder",
		FooterGit:     "placeholder",
		FooterContext: "placeholder",
	}
}

// RenderPlain renders the static shell without terminal styling.
func RenderPlain(state ViewState, size Size) string {
	return renderProduct(state, size, false)
}

// RenderANSI renders the static shell with stable ANSI styling.
func RenderANSI(state ViewState, size Size) string {
	return renderProduct(state, size, true)
}

func renderProduct(state ViewState, size Size, ansi bool) string {
	size = normalizeSize(size)
	layout := layoutForSize(size)
	lines := make([]string, 0, size.Height)
	header := fitLine(state.AppName, sizeLabel(size), size.Width)
	status := fitLine(statusLine(state), "", size.Width)
	if ansi {
		header = ansiBold + header + ansiReset
		status = ansiDim + status + ansiReset
	}
	lines = append(lines, header, status)

	contentHeight := size.Height - 7
	if contentHeight < 8 {
		contentHeight = 8
	}
	if layout.RightRailVisible {
		leftWidth := size.Width - 44
		lines = append(lines, pairedPanelLines("Conversation", contentItems(state), leftWidth, "Session", rightRailSemanticItems(state), 42, contentHeight, ansi)...)
	} else {
		lines = append(lines, panelLines("Conversation", contentItems(state), size.Width, contentHeight, ansi)...)
	}
	lines = append(lines, promptPanelLines(state, size.Width, ansi)...)
	footer := fitLine("", "git: "+state.FooterGit+" | context: "+state.FooterContext+" | q quit", size.Width)
	if ansi {
		footer = ansiDim + footer + ansiReset
	}
	lines = append(lines, footer)
	if len(lines) > size.Height {
		lines = lines[:size.Height]
	}
	for len(lines) < size.Height {
		lines = append(lines, strings.Repeat(" ", size.Width))
	}
	return strings.Join(lines, "\n")
}

func sizeLabel(size Size) string {
	return fmt.Sprintf("%dx%d", size.Width, size.Height)
}

func statusLine(state ViewState) string {
	return "Stage " + state.Phase + " | Model " + state.PrimaryModel + " | Utility " + state.UtilityModel + " | Auto " + state.Autonomy
}

func contentItems(state ViewState) []string {
	var items []string
	if state.SurfaceTitle == "" {
		items = displayLabelLines(state)
	}
	items = append(items, chatLines(state.Transcript)...)
	items = append(items, surfaceLines(state.CommandRoute, state.RouteSource, state.SurfaceTitle, state.SurfaceLines)...)
	return items
}

func displayLabelLines(state ViewState) []string {
	if !hasDisplayLabelDetails(state) {
		return nil
	}
	return []string{
		"  Display labels:",
		"  primary model: " + state.PrimaryModel,
		"  utility model: " + state.UtilityModel,
		"  autonomy: " + state.Autonomy + " (display-only)",
		"",
	}
}

func hasDisplayLabelDetails(state ViewState) bool {
	return state.PrimaryModel != "placeholder" || state.UtilityModel != "placeholder" || state.Autonomy != "placeholder"
}

func panelLines(title string, items []string, width int, height int, ansi bool) []string {
	if width < 20 {
		width = 20
	}
	if height < 3 {
		height = 3
	}
	lines := []string{panelTop(title, width, ansi)}
	contentHeight := height - 2
	for i := 0; i < contentHeight; i++ {
		text := ""
		if i < len(items) {
			text = strings.TrimPrefix(items[i], "  ")
		}
		lines = append(lines, panelRow(text, width))
	}
	lines = append(lines, panelBottom(width))
	return lines
}

func pairedPanelLines(leftTitle string, leftItems []string, leftWidth int, rightTitle string, rightItems []string, rightWidth int, height int, ansi bool) []string {
	left := panelLines(leftTitle, leftItems, leftWidth, height, ansi)
	right := panelLines(rightTitle, rightItems, rightWidth, height, ansi)
	lines := make([]string, 0, height)
	for i := 0; i < height; i++ {
		lines = append(lines, left[i]+"  "+right[i])
	}
	return lines
}

func promptPanelLines(state ViewState, width int, ansi bool) []string {
	input := promptLine(state.PromptInput)
	if ansi {
		input = ansiCyan + input + ansiReset
	}
	return []string{
		panelTop("Prompt", width, ansi),
		panelRow(input, width),
		panelBottom(width),
	}
}

func panelTop(title string, width int, ansi bool) string {
	label := " " + title + " "
	if ansi {
		label = " " + ansiYellow + title + ansiReset + " "
	}
	return "+" + fitVisible(label, width-2, "-") + "+"
}

func panelBottom(width int) string {
	return "+" + strings.Repeat("-", width-2) + "+"
}

func panelRow(text string, width int) string {
	return "| " + fitVisible(text, width-4, " ") + " |"
}

func fitLine(left string, right string, width int) string {
	left = trimVisible(left, width)
	right = trimVisible(right, width)
	space := width - visibleLen(left) - visibleLen(right)
	if space < 1 {
		return trimVisible(left+" "+right, width)
	}
	return left + strings.Repeat(" ", space) + right
}

func fitVisible(text string, width int, pad string) string {
	text = trimVisible(text, width)
	return text + strings.Repeat(pad, width-visibleLen(text))
}

func trimVisible(text string, width int) string {
	if visibleLen(text) <= width {
		return text
	}
	if width <= 1 {
		if width < 1 {
			return ""
		}
		return "."
	}
	plain := stripANSI(text)
	return plain[:width-1] + "~"
}

func visibleLen(text string) int {
	return len(stripANSI(text))
}

func stripANSI(text string) string {
	for _, code := range []string{ansiBold, ansiDim, ansiCyan, ansiYellow, ansiReset} {
		text = strings.ReplaceAll(text, code, "")
	}
	return text
}

// SemanticSnapshot is the agent-readable meaning of the rendered static shell.
type SemanticSnapshot struct {
	Scenario string           `json:"scenario"`
	Screen   SemanticScreen   `json:"screen"`
	Layout   SemanticLayout   `json:"layout"`
	Session  SemanticSession  `json:"session"`
	Command  *SemanticCommand `json:"command,omitempty"`
	Regions  []SemanticRegion `json:"regions"`
	Actions  []SemanticAction `json:"actions"`
}

// SemanticScreen describes the terminal surface for a snapshot.
type SemanticScreen struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Focus  string `json:"focus"`
}

// SemanticLayout describes deterministic presentation layout metadata.
type SemanticLayout struct {
	Class            LayoutClass `json:"class"`
	RightRailVisible bool        `json:"right_rail_visible"`
}

// SemanticSession describes session-level presentation state.
type SemanticSession struct {
	Phase              string `json:"phase"`
	PhaseSource        string `json:"phase_source"`
	WorkflowTransition bool   `json:"workflow_transition"`
	Active             bool   `json:"active"`
	QueuedMessages     int    `json:"queued_messages"`
	PrimaryModel       string `json:"primary_model"`
	UtilityModel       string `json:"utility_model"`
	Autonomy           string `json:"autonomy"`
}

// SemanticCommand describes a visible command surface without implying execution.
type SemanticCommand struct {
	Route              string `json:"route"`
	RouteSource        string `json:"route_source"`
	Surface            string `json:"surface"`
	Visible            bool   `json:"visible"`
	Executed           bool   `json:"executed"`
	WorkflowTransition bool   `json:"workflow_transition"`
}

// SemanticRegion describes a visible region of the static shell.
type SemanticRegion struct {
	Name    string   `json:"name"`
	Visible bool     `json:"visible"`
	Items   []string `json:"items"`
}

// SemanticAction describes a user-visible action in the static shell.
type SemanticAction struct {
	Name  string `json:"name"`
	Input string `json:"input"`
}

// Semantic returns the semantic snapshot for a static shell render.
func Semantic(state ViewState, size Size) SemanticSnapshot {
	size = normalizeSize(size)
	layout := layoutForSize(size)
	regions := []SemanticRegion{
		{Name: "header", Visible: true, Items: []string{state.AppName}},
		{Name: "phase", Visible: true, Items: []string{state.Phase, "display-only"}},
		{Name: "model", Visible: true, Items: []string{"primary: " + state.PrimaryModel, "utility: " + state.UtilityModel, "autonomy: " + state.Autonomy}},
		{Name: "chat", Visible: true, Items: semanticChatItems(state.Transcript)},
	}
	if hasDisplayLabelDetails(state) {
		regions = append(regions, SemanticRegion{Name: "display_labels", Visible: true, Items: semanticDisplayLabelItems(state)})
	}
	if state.SurfaceTitle != "" {
		regions = append(regions, SemanticRegion{Name: "command", Visible: true, Items: semanticSurfaceItems(state.CommandRoute, state.RouteSource, state.SurfaceTitle, state.SurfaceLines)})
	}
	var command *SemanticCommand
	if state.CommandRoute != "" || state.SurfaceTitle != "" {
		command = &SemanticCommand{
			Route:              state.CommandRoute,
			RouteSource:        state.RouteSource,
			Surface:            state.SurfaceTitle,
			Visible:            state.SurfaceTitle != "",
			Executed:           false,
			WorkflowTransition: false,
		}
	}
	regions = append(regions,
		SemanticRegion{Name: "prompt", Visible: true, Items: []string{promptLine(state.PromptInput)}},
		SemanticRegion{Name: "footer", Visible: true, Items: []string{"git: " + state.FooterGit, "context: " + state.FooterContext, "quit: q"}},
	)
	if layout.RightRailVisible {
		regions = append(regions, SemanticRegion{Name: "right_rail", Visible: true, Items: rightRailSemanticItems(state)})
	}
	return SemanticSnapshot{
		Scenario: state.Scenario,
		Screen: SemanticScreen{
			Width:  size.Width,
			Height: size.Height,
			Focus:  "prompt",
		},
		Layout: SemanticLayout{
			Class:            layout.Class,
			RightRailVisible: layout.RightRailVisible,
		},
		Session: SemanticSession{
			Phase:              state.Phase,
			PhaseSource:        state.PhaseSource,
			WorkflowTransition: false,
			Active:             false,
			QueuedMessages:     0,
			PrimaryModel:       state.PrimaryModel,
			UtilityModel:       state.UtilityModel,
			Autonomy:           state.Autonomy,
		},
		Command: command,
		Regions: regions,
		Actions: []SemanticAction{
			{Name: "quit", Input: "q"},
		},
	}
}

func promptLine(input string) string {
	if input == "" {
		return ">"
	}
	return "> " + input
}

func chatLines(transcript []TranscriptTurn) []string {
	if len(transcript) == 0 {
		return []string{"  No messages yet."}
	}
	lines := make([]string, 0, len(transcript)*2)
	for _, turn := range transcript {
		lines = append(lines, "  user: "+turn.UserText, "  assistant: "+turn.AssistantText)
	}
	return lines
}

func semanticChatItems(transcript []TranscriptTurn) []string {
	if len(transcript) == 0 {
		return []string{"No messages yet."}
	}
	items := make([]string, 0, len(transcript)*2)
	for _, turn := range transcript {
		items = append(items, "user: "+turn.UserText, "assistant: "+turn.AssistantText)
	}
	return items
}

func semanticDisplayLabelItems(state ViewState) []string {
	return []string{
		"primary model: " + state.PrimaryModel,
		"utility model: " + state.UtilityModel,
		"autonomy: " + state.Autonomy,
		"display-only",
	}
}

func surfaceLines(route string, source string, title string, items []string) []string {
	if title == "" {
		return nil
	}
	lines := []string{"", title + ":"}
	if route != "" {
		lines = append(lines, "  command route: "+route)
	}
	if source != "" {
		lines = append(lines, "  route source: "+source)
	}
	for _, item := range items {
		lines = append(lines, "  "+item)
	}
	return lines
}

func semanticSurfaceItems(route string, source string, title string, items []string) []string {
	if title == "" {
		return nil
	}
	result := make([]string, 0, len(items)+3)
	result = append(result, title)
	if route != "" {
		result = append(result, "command route: "+route)
	}
	if source != "" {
		result = append(result, "route source: "+source)
	}
	result = append(result, items...)
	return result
}

func rightRailSemanticItems(state ViewState) []string {
	return []string{
		"phase source: " + state.PhaseSource,
		"primary model: " + state.PrimaryModel,
		"utility model: " + state.UtilityModel,
		"autonomy: " + state.Autonomy,
		"git: " + state.FooterGit,
		"context: " + state.FooterContext,
	}
}

// RenderSemanticJSON renders an indented semantic JSON snapshot.
func RenderSemanticJSON(state ViewState, size Size) string {
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(Semantic(state, size)); err != nil {
		panic(fmt.Sprintf("marshal semantic snapshot: %v", err))
	}
	return strings.TrimSuffix(data.String(), "\n")
}

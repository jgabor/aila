package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	ansiBold            = "\x1b[1m"
	ansiDim             = "\x1b[2m"
	ansiCyan            = "\x1b[36m"
	ansiYellow          = "\x1b[33m"
	ansiReset           = "\x1b[0m"
	maxDisplayTextBytes = 240
)

var (
	secretLikeText = regexp.MustCompile(`(?i)(bearer\s+)[^\s,;]+|((?:api[_-]?key|token|password|secret)\s*[:=]\s*)[^\s,;]+`)
	pathLikeText   = regexp.MustCompile(`(?i)(~|/home/[^\s,;]+|/tmp/[^\s,;]+|[^\s,;]*(?:\x2eaila|\x2econfig|project\.toml|artifacts/|indexes/)[^\s,;]*|[a-z]:\\[^\s,;]+|\\\\[^\s,;]+)`)
)

// ViewState is the deterministic data rendered by the M2 static shell.
type ViewState struct {
	Scenario           string
	AppName            string
	Phase              string
	PhaseSource        string
	RuntimeStatus      string
	StatusSource       string
	StatusDetail       string
	RuntimeActive      bool
	RuntimeResult      string
	QueuedCount        int
	QueuedText         []string
	PrimaryModel       string
	UtilityModel       string
	Autonomy           string
	ProjectStoreStatus string
	ProjectStoreSource string
	ProjectStoreDetail string
	MemorySource       string
	MemorySessionID    string
	MemoryBlockers     []string
	MemoryConcerns     []string
	Diagnostics        []DiagnosticView
	FooterGit          string
	FooterContext      string
	Transcript         []TranscriptTurn
	CommandRoute       string
	RouteSource        string
	SurfaceTitle       string
	SurfaceLines       []string
	HistoryItems       []HistoryItem
	HistorySelected    int
	HistoryFocus       bool
	HistoryEmpty       bool
	PromptInput        string
}

// HistoryItem is app-injected read-only history display data.
type HistoryItem struct {
	EventID     string
	RunID       string
	SessionID   string
	Kind        string
	Source      string
	Provenance  string
	DisplayText string
}

// DiagnosticView is app-owned diagnostic presentation data consumed by the TUI.
type DiagnosticView struct {
	Severity         string `json:"severity"`
	Source           string `json:"source"`
	RecoveryAction   string `json:"recovery_action"`
	AffectedArtifact string `json:"affected_artifact"`
	UserInputNeeded  bool   `json:"user_input_needed"`
	BoundedMessage   string `json:"bounded_message"`
}

// IdleEmptyState returns the static first-launch view state.
func IdleEmptyState() ViewState {
	return ViewState{
		Scenario:      "idle-empty",
		AppName:       "Aila",
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
	status := "Stage " + state.Phase
	if state.RuntimeStatus != "" {
		status += " | Runtime " + safeText(state.RuntimeStatus)
	}
	return status + " | Model " + state.PrimaryModel + " | Utility " + state.UtilityModel + " | Auto " + state.Autonomy
}

func contentItems(state ViewState) []string {
	var items []string
	if state.SurfaceTitle == "" {
		items = displayLabelLines(state)
	}
	items = append(items, runtimeStatusLines(state)...)
	items = append(items, memoryLines(state)...)
	items = append(items, queueLines(state)...)
	items = append(items, chatLines(state.Transcript)...)
	items = append(items, surfaceLines(state.CommandRoute, state.RouteSource, state.SurfaceTitle, state.SurfaceLines)...)
	return items
}

func memoryLines(state ViewState) []string {
	if !hasMemory(state) {
		return nil
	}
	lines := []string{
		"  Resumed memory:",
		"  source: " + safeText(state.MemorySource),
		"  session id: " + safeText(state.MemorySessionID),
		fmt.Sprintf("  resumed transcript turns: %d", len(state.Transcript)),
		fmt.Sprintf("  queued count: %d", state.QueuedCount),
		fmt.Sprintf("  diagnostics: %d", len(state.Diagnostics)),
	}
	for _, blocker := range state.MemoryBlockers {
		lines = append(lines, "  blocker: "+safeText(blocker))
	}
	for _, concern := range state.MemoryConcerns {
		lines = append(lines, "  concern: "+safeText(concern))
	}
	return append(lines, "")
}

func hasMemory(state ViewState) bool {
	return state.MemorySource != "" || state.MemorySessionID != "" || len(state.MemoryBlockers) > 0 || len(state.MemoryConcerns) > 0
}

func runtimeStatusLines(state ViewState) []string {
	if state.RuntimeStatus == "" {
		return nil
	}
	lines := []string{
		"  Runtime status:",
		"  status: " + safeText(state.RuntimeStatus),
	}
	if state.StatusSource != "" {
		lines = append(lines, "  status source: "+safeText(state.StatusSource))
	}
	if state.StatusDetail != "" {
		lines = append(lines, "  detail: "+safeText(state.StatusDetail))
	}
	lines = append(lines, "  active: "+boolLabel(state.RuntimeActive))
	if state.RuntimeResult != "" {
		lines = append(lines, "  result: "+safeText(state.RuntimeResult))
	}
	lines = append(lines, interruptStatusLines(state)...)
	lines = append(lines, "")
	return lines
}

func interruptStatusLines(state ViewState) []string {
	if !hasInterruptState(state) {
		return nil
	}
	lines := []string{
		"  interrupt state:",
		"  interrupt status: " + state.RuntimeStatus,
		"  lower-layer cancellation executed: false",
	}
	if state.RuntimeStatus == "canceling" {
		lines = append(lines, "  interrupt outcome: pending")
	}
	if state.RuntimeStatus == "canceled" {
		lines = append(lines, "  interrupt outcome: fake work canceled")
	}
	return lines
}

func hasInterruptState(state ViewState) bool {
	return state.RuntimeStatus == "canceling" || state.RuntimeStatus == "canceled"
}

func queueLines(state ViewState) []string {
	if state.QueuedCount <= 0 {
		return nil
	}
	lines := []string{
		"  Queued input:",
		fmt.Sprintf("  queued messages: %d", state.QueuedCount),
		"  default action: send after current turn",
		"  action status: presentation-only; not executed by the TUI",
	}
	for _, text := range state.QueuedText {
		lines = append(lines, "  queued: "+safeText(text))
	}
	lines = append(lines, "")
	return lines
}

func displayLabelLines(state ViewState) []string {
	if !hasDisplayLabelDetails(state) {
		return nil
	}
	lines := []string{
		"  Display labels:",
		"  primary model: " + state.PrimaryModel,
		"  utility model: " + state.UtilityModel,
		"  autonomy: " + state.Autonomy + " (display-only)",
	}
	if hasProjectStoreStatus(state) {
		line := "  project store: " + state.ProjectStoreStatus
		if state.ProjectStoreDetail != "" {
			line += " - " + state.ProjectStoreDetail
		}
		lines = append(lines, line)
	}
	lines = append(lines, diagnosticLines(state.Diagnostics)...)
	return append(lines, "")
}

func diagnosticLines(diagnostics []DiagnosticView) []string {
	if len(diagnostics) == 0 {
		return nil
	}
	lines := []string{"  Diagnostics:"}
	for _, diagnostic := range diagnostics {
		lines = append(lines,
			"  severity: "+diagnostic.Severity,
			"  source: "+safeText(diagnostic.Source),
			"  affected artifact: "+diagnostic.AffectedArtifact,
			"  recovery action: "+diagnostic.RecoveryAction,
			"  user input needed: "+boolLabel(diagnostic.UserInputNeeded),
			"  message: "+safeText(diagnostic.BoundedMessage),
		)
	}
	return lines
}

func hasDisplayLabelDetails(state ViewState) bool {
	return state.PrimaryModel != "placeholder" || state.UtilityModel != "placeholder" || state.Autonomy != "placeholder" || hasProjectStoreStatus(state) || len(state.Diagnostics) > 0
}

func hasProjectStoreStatus(state ViewState) bool {
	return state.ProjectStoreStatus != ""
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
	Scenario    string               `json:"scenario"`
	Screen      SemanticScreen       `json:"screen"`
	Layout      SemanticLayout       `json:"layout"`
	Session     SemanticSession      `json:"session"`
	Memory      *SemanticMemory      `json:"memory,omitempty"`
	Diagnostics []SemanticDiagnostic `json:"diagnostics,omitempty"`
	Interrupt   *SemanticInterrupt   `json:"interrupt,omitempty"`
	Command     *SemanticCommand     `json:"command,omitempty"`
	History     *SemanticHistory     `json:"history,omitempty"`
	Regions     []SemanticRegion     `json:"regions"`
	Actions     []SemanticAction     `json:"actions"`
}

// SemanticMemory describes app-injected resumed current-session memory.
type SemanticMemory struct {
	Source          string   `json:"source"`
	SessionID       string   `json:"session_id"`
	TranscriptTurns int      `json:"transcript_turns"`
	QueuedCount     int      `json:"queued_count"`
	Blockers        []string `json:"blockers,omitempty"`
	Concerns        []string `json:"concerns,omitempty"`
	Diagnostics     int      `json:"diagnostics"`
}

// SemanticDiagnostic is the stable diagnostic status contract for fixtures.
type SemanticDiagnostic struct {
	Severity         string `json:"severity"`
	Source           string `json:"source"`
	RecoveryAction   string `json:"recovery_action"`
	AffectedArtifact string `json:"affected_artifact"`
	UserInputNeeded  bool   `json:"user_input_needed"`
	BoundedMessage   string `json:"bounded_message"`
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
	RuntimeStatus      string `json:"runtime_status,omitempty"`
	StatusSource       string `json:"status_source,omitempty"`
	StatusDetail       string `json:"status_detail,omitempty"`
	RuntimeResult      string `json:"runtime_result,omitempty"`
	Active             bool   `json:"active"`
	QueuedMessages     int    `json:"queued_messages"`
	PrimaryModel       string `json:"primary_model"`
	UtilityModel       string `json:"utility_model"`
	Autonomy           string `json:"autonomy"`
	ProjectStoreStatus string `json:"project_store_status,omitempty"`
	ProjectStoreSource string `json:"project_store_source,omitempty"`
	ProjectStoreDetail string `json:"project_store_detail,omitempty"`
}

// SemanticInterrupt describes injected interrupt display state without implying
// lower-layer IO cancellation.
type SemanticInterrupt struct {
	State                          string `json:"state"`
	Outcome                        string `json:"outcome,omitempty"`
	LowerLayerCancellationExecuted bool   `json:"lower_layer_cancellation_executed"`
}

// SemanticCommand describes a visible command surface without implying execution.
type SemanticCommand struct {
	Route       string `json:"route"`
	RouteSource string `json:"route_source"`
	Surface     string `json:"surface"`
	Visible     bool   `json:"visible"`
	Executed    bool   `json:"executed"`
}

// SemanticHistory describes app-injected read-only history presentation state.
type SemanticHistory struct {
	Visible       bool                  `json:"visible"`
	ReadOnly      bool                  `json:"read_only"`
	UndoEnabled   bool                  `json:"undo_enabled"`
	Focus         bool                  `json:"focus"`
	Empty         bool                  `json:"empty"`
	Count         int                   `json:"count"`
	SelectedIndex int                   `json:"selected_index"`
	SelectedID    string                `json:"selected_id,omitempty"`
	Items         []SemanticHistoryItem `json:"items"`
}

// SemanticHistoryItem describes one app-injected history row.
type SemanticHistoryItem struct {
	EventID     string `json:"event_id"`
	RunID       string `json:"run_id"`
	SessionID   string `json:"session_id"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	Provenance  string `json:"provenance"`
	DisplayText string `json:"display_text"`
	Selected    bool   `json:"selected"`
}

// SemanticRegion describes a visible region of the static shell.
type SemanticRegion struct {
	Name    string   `json:"name"`
	Visible bool     `json:"visible"`
	Items   []string `json:"items"`
}

// SemanticAction describes a user-visible action in the static shell.
type SemanticAction struct {
	Name             string `json:"name"`
	Input            string `json:"input"`
	Default          bool   `json:"default,omitempty"`
	PresentationOnly bool   `json:"presentation_only,omitempty"`
	Executed         bool   `json:"executed,omitempty"`
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
	if hasProjectStoreStatus(state) {
		regions = append(regions, SemanticRegion{Name: "project_store", Visible: true, Items: semanticProjectStoreItems(state)})
	}
	if len(state.Diagnostics) > 0 {
		regions = append(regions, SemanticRegion{Name: "diagnostics", Visible: true, Items: semanticDiagnosticItems(state.Diagnostics)})
	}
	if hasMemory(state) {
		regions = append(regions, SemanticRegion{Name: "memory", Visible: true, Items: semanticMemoryItems(state)})
	}
	if state.RuntimeStatus != "" {
		regions = append(regions, SemanticRegion{Name: "runtime_status", Visible: true, Items: semanticRuntimeStatusItems(state)})
	}
	if hasInterruptState(state) {
		regions = append(regions, SemanticRegion{Name: "interrupt", Visible: true, Items: semanticInterruptItems(state)})
	}
	if state.QueuedCount > 0 {
		regions = append(regions, SemanticRegion{Name: "queue", Visible: true, Items: semanticQueueItems(state)})
	}
	if state.SurfaceTitle != "" {
		regions = append(regions, SemanticRegion{Name: "command", Visible: true, Items: semanticSurfaceItems(state.CommandRoute, state.RouteSource, state.SurfaceTitle, state.SurfaceLines)})
	}
	if historyVisible(state) {
		regions = append(regions, SemanticRegion{Name: "history", Visible: true, Items: semanticHistoryRegionItems(state)})
	}
	var command *SemanticCommand
	if state.CommandRoute != "" || state.SurfaceTitle != "" {
		command = &SemanticCommand{
			Route:       state.CommandRoute,
			RouteSource: state.RouteSource,
			Surface:     state.SurfaceTitle,
			Visible:     state.SurfaceTitle != "",
			Executed:    false,
		}
	}
	regions = append(regions,
		SemanticRegion{Name: "prompt", Visible: true, Items: []string{promptLine(state.PromptInput)}},
		SemanticRegion{Name: "footer", Visible: true, Items: []string{"git: " + state.FooterGit, "context: " + state.FooterContext, "quit: q"}},
	)
	if layout.RightRailVisible {
		regions = append(regions, SemanticRegion{Name: "right_rail", Visible: true, Items: rightRailSemanticItems(state)})
	}
	actions := []SemanticAction{{Name: "quit", Input: "q"}}
	if state.QueuedCount > 0 {
		actions = append(actions, SemanticAction{
			Name:             "queue_after_current_turn",
			Input:            "enter",
			Default:          true,
			PresentationOnly: true,
			Executed:         false,
		})
	}
	snapshot := SemanticSnapshot{
		Scenario: state.Scenario,
		Screen: SemanticScreen{
			Width:  size.Width,
			Height: size.Height,
			Focus:  semanticFocus(state),
		},
		Layout: SemanticLayout{
			Class:            layout.Class,
			RightRailVisible: layout.RightRailVisible,
		},
		Session: SemanticSession{
			Phase:              state.Phase,
			PhaseSource:        state.PhaseSource,
			RuntimeStatus:      safeText(state.RuntimeStatus),
			StatusSource:       safeText(state.StatusSource),
			StatusDetail:       safeText(state.StatusDetail),
			RuntimeResult:      safeText(state.RuntimeResult),
			Active:             state.RuntimeActive,
			QueuedMessages:     state.QueuedCount,
			PrimaryModel:       state.PrimaryModel,
			UtilityModel:       state.UtilityModel,
			Autonomy:           state.Autonomy,
			ProjectStoreStatus: state.ProjectStoreStatus,
			ProjectStoreSource: state.ProjectStoreSource,
			ProjectStoreDetail: state.ProjectStoreDetail,
		},
		Memory:      semanticMemory(state),
		Diagnostics: semanticDiagnostics(state.Diagnostics),
		Command:     command,
		History:     semanticHistory(state),
		Regions:     regions,
		Actions:     actions,
	}
	if hasInterruptState(state) {
		snapshot.Interrupt = semanticInterrupt(state)
	}
	return snapshot
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
		if turn.UserText != "" {
			lines = append(lines, "  user: "+safeText(turn.UserText))
		}
		if turn.AssistantText != "" {
			lines = append(lines, "  assistant: "+safeText(turn.AssistantText))
		}
	}
	return lines
}

func semanticChatItems(transcript []TranscriptTurn) []string {
	if len(transcript) == 0 {
		return []string{"No messages yet."}
	}
	items := make([]string, 0, len(transcript)*2)
	for _, turn := range transcript {
		if turn.UserText != "" {
			items = append(items, "user: "+safeText(turn.UserText))
		}
		if turn.AssistantText != "" {
			items = append(items, "assistant: "+safeText(turn.AssistantText))
		}
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

func semanticProjectStoreItems(state ViewState) []string {
	items := []string{"status: " + state.ProjectStoreStatus}
	if state.ProjectStoreSource != "" {
		items = append(items, "source: "+state.ProjectStoreSource)
	}
	if state.ProjectStoreDetail != "" {
		items = append(items, "detail: "+state.ProjectStoreDetail)
	}
	items = append(items, "app-owned")
	return items
}

func semanticDiagnostics(diagnostics []DiagnosticView) []SemanticDiagnostic {
	if len(diagnostics) == 0 {
		return nil
	}
	items := make([]SemanticDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		items = append(items, SemanticDiagnostic{
			Severity:         safeText(diagnostic.Severity),
			Source:           safeText(diagnostic.Source),
			RecoveryAction:   safeText(diagnostic.RecoveryAction),
			AffectedArtifact: safeText(diagnostic.AffectedArtifact),
			UserInputNeeded:  diagnostic.UserInputNeeded,
			BoundedMessage:   safeText(diagnostic.BoundedMessage),
		})
	}
	return items
}

func semanticDiagnosticItems(diagnostics []DiagnosticView) []string {
	items := make([]string, 0, len(diagnostics)*6)
	for _, diagnostic := range diagnostics {
		items = append(items,
			"severity: "+diagnostic.Severity,
			"source: "+safeText(diagnostic.Source),
			"affected_artifact: "+diagnostic.AffectedArtifact,
			"recovery_action: "+diagnostic.RecoveryAction,
			"user_input_needed: "+boolLabel(diagnostic.UserInputNeeded),
			"bounded_message: "+safeText(diagnostic.BoundedMessage),
		)
	}
	items = append(items, "app-owned", "display-only")
	return items
}

func semanticMemory(state ViewState) *SemanticMemory {
	if !hasMemory(state) {
		return nil
	}
	return &SemanticMemory{
		Source:          safeText(state.MemorySource),
		SessionID:       safeText(state.MemorySessionID),
		TranscriptTurns: len(state.Transcript),
		QueuedCount:     state.QueuedCount,
		Blockers:        safeTextSlice(state.MemoryBlockers),
		Concerns:        safeTextSlice(state.MemoryConcerns),
		Diagnostics:     len(state.Diagnostics),
	}
}

func semanticMemoryItems(state ViewState) []string {
	memory := semanticMemory(state)
	items := []string{
		"source: " + memory.Source,
		"session_id: " + memory.SessionID,
		fmt.Sprintf("transcript_turns: %d", memory.TranscriptTurns),
		fmt.Sprintf("queued_count: %d", memory.QueuedCount),
		fmt.Sprintf("diagnostics: %d", memory.Diagnostics),
		"app-owned",
		"display-only",
	}
	for _, blocker := range memory.Blockers {
		items = append(items, "blocker: "+blocker)
	}
	for _, concern := range memory.Concerns {
		items = append(items, "concern: "+concern)
	}
	return items
}

func semanticRuntimeStatusItems(state ViewState) []string {
	items := []string{"status: " + safeText(state.RuntimeStatus)}
	if state.StatusSource != "" {
		items = append(items, "status source: "+safeText(state.StatusSource))
	}
	if state.StatusDetail != "" {
		items = append(items, "detail: "+safeText(state.StatusDetail))
	}
	items = append(items, "active: "+boolLabel(state.RuntimeActive))
	if state.RuntimeResult != "" {
		items = append(items, "result: "+safeText(state.RuntimeResult))
	}
	items = append(items, interruptStatusLines(state)...)
	items = append(items, "display-only")
	return items
}

func semanticInterruptItems(state ViewState) []string {
	interrupt := semanticInterrupt(state)
	items := []string{
		"state: " + interrupt.State,
		"lower_layer_cancellation_executed: false",
		"display-only",
	}
	if interrupt.Outcome != "" {
		items = append(items, "outcome: "+interrupt.Outcome)
	}
	return items
}

func semanticInterrupt(state ViewState) *SemanticInterrupt {
	interrupt := &SemanticInterrupt{
		State:                          state.RuntimeStatus,
		LowerLayerCancellationExecuted: false,
	}
	if state.RuntimeStatus == "canceling" {
		interrupt.Outcome = "pending"
	}
	if state.RuntimeStatus == "canceled" {
		interrupt.Outcome = "fake work canceled"
	}
	return interrupt
}

func semanticQueueItems(state ViewState) []string {
	items := []string{
		fmt.Sprintf("queued messages: %d", state.QueuedCount),
		"default action: send after current turn",
		"presentation-only",
		"executed: false",
	}
	for _, text := range state.QueuedText {
		items = append(items, "queued: "+safeText(text))
	}
	return items
}

func safeTextSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, safeText(value))
	}
	return items
}

func safeText(value string) string {
	value = stripTerminalControls(value)
	value = secretLikeText.ReplaceAllString(value, "[redacted]")
	value = pathLikeText.ReplaceAllString(value, "[path-redacted]")
	value = strings.Join(strings.Fields(value), " ")
	return limitTextBytes(value, maxDisplayTextBytes)
}

func semanticFocus(state ViewState) string {
	if historyVisible(state) && state.HistoryFocus {
		return "history"
	}
	return "prompt"
}

func stripTerminalControls(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); {
		r, size := utf8.DecodeRuneInString(value[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		if r == '\x1b' {
			i += size
			if i < len(value) && value[i] == '[' {
				i++
			}
			for i < len(value) {
				r, size = utf8.DecodeRuneInString(value[i:])
				i += size
				if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
					break
				}
			}
			continue
		}
		if r < ' ' || r == '\x7f' {
			out.WriteByte(' ')
			i += size
			continue
		}
		out.WriteRune(r)
		i += size
	}
	return out.String()
}

func limitTextBytes(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	if maxBytes <= 1 {
		return ""
	}
	limit := maxBytes - 1
	for !utf8.ValidString(value[:limit]) {
		limit--
	}
	return value[:limit] + "~"
}

func boolLabel(value bool) string {
	if value {
		return "true"
	}
	return "false"
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

func historySurfaceLines(state ViewState) []string {
	if !historyVisible(state) {
		return nil
	}
	if state.HistoryEmpty || len(state.HistoryItems) == 0 {
		return []string{
			"read-only: true",
			"empty history",
			"no fake history events recorded yet",
		}
	}
	selected := clampHistorySelection(state)
	lines := []string{
		"read-only: true",
		fmt.Sprintf("entries: %d", len(state.HistoryItems)),
		fmt.Sprintf("selected: %d", selected+1),
	}
	start := historyWindowStart(state, 12)
	for index, item := range visibleHistoryItems(state, 12) {
		marker := " "
		absolute := start + index
		if absolute == selected {
			marker = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %s %s %s %s %s", marker, safeText(item.RunID), safeText(item.SessionID), safeText(item.EventID), safeText(item.Kind), safeText(item.DisplayText)))
	}
	item := state.HistoryItems[selected]
	lines = append(lines,
		"selected event id: "+safeText(item.EventID),
		"selected run id: "+safeText(item.RunID),
		"selected session id: "+safeText(item.SessionID),
		"selected kind: "+safeText(item.Kind),
		"selected source: "+safeText(item.Source),
		"selected provenance: "+safeText(item.Provenance),
		"selected text: "+safeText(item.DisplayText),
	)
	return lines
}

func historyVisible(state ViewState) bool {
	return state.SurfaceTitle == "history" || state.CommandRoute == "history" || state.HistoryFocus
}

func clampHistorySelection(state ViewState) int {
	if len(state.HistoryItems) == 0 {
		return 0
	}
	if state.HistorySelected < 0 {
		return 0
	}
	if state.HistorySelected >= len(state.HistoryItems) {
		return len(state.HistoryItems) - 1
	}
	return state.HistorySelected
}

func visibleHistoryItems(state ViewState, limit int) []HistoryItem {
	if limit <= 0 || len(state.HistoryItems) <= limit {
		return state.HistoryItems
	}
	start := historyWindowStart(state, limit)
	return state.HistoryItems[start : start+limit]
}

func historyWindowStart(state ViewState, limit int) int {
	if limit <= 0 || len(state.HistoryItems) <= limit {
		return 0
	}
	selected := clampHistorySelection(state)
	start := selected - limit/2
	if start < 0 {
		return 0
	}
	maxStart := len(state.HistoryItems) - limit
	if start > maxStart {
		return maxStart
	}
	return start
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

func semanticHistory(state ViewState) *SemanticHistory {
	if !historyVisible(state) {
		return nil
	}
	selected := clampHistorySelection(state)
	items := make([]SemanticHistoryItem, 0, len(state.HistoryItems))
	for index, item := range state.HistoryItems {
		items = append(items, SemanticHistoryItem{
			EventID:     safeText(item.EventID),
			RunID:       safeText(item.RunID),
			SessionID:   safeText(item.SessionID),
			Kind:        safeText(item.Kind),
			Source:      safeText(item.Source),
			Provenance:  safeText(item.Provenance),
			DisplayText: safeText(item.DisplayText),
			Selected:    index == selected && len(state.HistoryItems) > 0,
		})
	}
	selectedID := ""
	if len(state.HistoryItems) > 0 {
		selectedID = safeText(state.HistoryItems[selected].EventID)
	}
	return &SemanticHistory{
		Visible:       true,
		ReadOnly:      true,
		UndoEnabled:   false,
		Focus:         state.HistoryFocus,
		Empty:         state.HistoryEmpty || len(state.HistoryItems) == 0,
		Count:         len(state.HistoryItems),
		SelectedIndex: selected,
		SelectedID:    selectedID,
		Items:         items,
	}
}

func semanticHistoryRegionItems(state ViewState) []string {
	history := semanticHistory(state)
	if history == nil {
		return nil
	}
	items := []string{
		"read_only: true",
		"undo_enabled: false",
		"focus: " + boolLabel(history.Focus),
		"empty: " + boolLabel(history.Empty),
		fmt.Sprintf("count: %d", history.Count),
		fmt.Sprintf("selected_index: %d", history.SelectedIndex),
	}
	if history.SelectedID != "" {
		items = append(items, "selected_id: "+history.SelectedID)
	}
	for _, item := range history.Items {
		items = append(items, "item: "+item.RunID+" "+item.SessionID+" "+item.EventID+" "+item.Kind+" "+item.DisplayText+" selected: "+boolLabel(item.Selected))
	}
	items = append(items, "app-owned", "display-only")
	return items
}

func rightRailSemanticItems(state ViewState) []string {
	items := []string{
		"phase source: " + state.PhaseSource,
		"primary model: " + state.PrimaryModel,
		"utility model: " + state.UtilityModel,
		"autonomy: " + state.Autonomy,
	}
	if hasProjectStoreStatus(state) {
		items = append(items, semanticProjectStoreItems(state)...)
	}
	if state.RuntimeStatus != "" {
		items = append(items, semanticRuntimeStatusItems(state)...)
	}
	if state.QueuedCount > 0 {
		items = append(items, semanticQueueItems(state)...)
	}
	items = append(items, "git: "+state.FooterGit, "context: "+state.FooterContext)
	return items
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

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
	size = normalizeSize(size)
	return strings.Join([]string{
		state.AppName,
		fmt.Sprintf("screen: %dx%d", size.Width, size.Height),
		fmt.Sprintf("phase: %s (display-only)", state.Phase),
		fmt.Sprintf("model: %s | utility: %s | autonomy: %s", state.PrimaryModel, state.UtilityModel, state.Autonomy),
		"",
		"chat:",
		"  No messages yet.",
		"",
		"prompt:",
		">",
		"",
		fmt.Sprintf("footer: git: %s | context: %s | quit: q", state.FooterGit, state.FooterContext),
	}, "\n")
}

// RenderANSI renders the static shell with stable ANSI styling.
func RenderANSI(state ViewState, size Size) string {
	size = normalizeSize(size)
	return strings.Join([]string{
		ansiBold + state.AppName + ansiReset,
		fmt.Sprintf("screen: %dx%d", size.Width, size.Height),
		"phase: " + ansiYellow + state.Phase + ansiReset + " (display-only)",
		"model: " + ansiCyan + state.PrimaryModel + ansiReset + " | utility: " + state.UtilityModel + " | autonomy: " + state.Autonomy,
		"",
		"chat:",
		"  No messages yet.",
		"",
		"prompt:",
		">",
		"",
		ansiDim + fmt.Sprintf("footer: git: %s | context: %s | quit: q", state.FooterGit, state.FooterContext) + ansiReset,
	}, "\n")
}

// SemanticSnapshot is the agent-readable meaning of the rendered static shell.
type SemanticSnapshot struct {
	Scenario string           `json:"scenario"`
	Screen   SemanticScreen   `json:"screen"`
	Session  SemanticSession  `json:"session"`
	Regions  []SemanticRegion `json:"regions"`
	Actions  []SemanticAction `json:"actions"`
}

// SemanticScreen describes the terminal surface for a snapshot.
type SemanticScreen struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Focus  string `json:"focus"`
}

// SemanticSession describes session-level presentation state.
type SemanticSession struct {
	Phase              string `json:"phase"`
	PhaseSource        string `json:"phase_source"`
	WorkflowTransition bool   `json:"workflow_transition"`
	Active             bool   `json:"active"`
	QueuedMessages     int    `json:"queued_messages"`
	Autonomy           string `json:"autonomy"`
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
	return SemanticSnapshot{
		Scenario: state.Scenario,
		Screen: SemanticScreen{
			Width:  size.Width,
			Height: size.Height,
			Focus:  "prompt",
		},
		Session: SemanticSession{
			Phase:              state.Phase,
			PhaseSource:        state.PhaseSource,
			WorkflowTransition: false,
			Active:             false,
			QueuedMessages:     0,
			Autonomy:           state.Autonomy,
		},
		Regions: []SemanticRegion{
			{Name: "header", Visible: true, Items: []string{state.AppName}},
			{Name: "phase", Visible: true, Items: []string{state.Phase, "display-only"}},
			{Name: "model", Visible: true, Items: []string{"primary: " + state.PrimaryModel, "utility: " + state.UtilityModel, "autonomy: " + state.Autonomy}},
			{Name: "chat", Visible: true, Items: []string{"No messages yet."}},
			{Name: "prompt", Visible: true, Items: []string{">"}},
			{Name: "footer", Visible: true, Items: []string{"git: " + state.FooterGit, "context: " + state.FooterContext, "quit: q"}},
		},
		Actions: []SemanticAction{
			{Name: "quit", Input: "q"},
		},
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

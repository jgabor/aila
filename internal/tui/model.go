package tui

import (
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jgabor/aila/internal/policy"
)

// PromptSubmitFunc routes non-empty submitted prompt text to the application layer.
type PromptSubmitFunc func(text string) TranscriptTurn

// CommandRouteFunc receives policy-owned command recommendations selected by the TUI.
type CommandRouteFunc func(policy.CommandRecommendation)

// TranscriptTurn is the presentation data for one submitted prompt and response.
type TranscriptTurn struct {
	UserText      string
	AssistantText string
}

// Size is the terminal dimensions used by the static M2 renderer.
type Size struct {
	Width  int
	Height int
}

// LayoutClass names the deterministic responsive layout bucket for a terminal size.
type LayoutClass string

const (
	LayoutCompact  LayoutClass = "compact"
	LayoutStandard LayoutClass = "standard"
	LayoutSpacious LayoutClass = "spacious"
	LayoutDesktop  LayoutClass = "desktop"
)

// LayoutState is presentation-only responsive state derived from terminal size.
type LayoutState struct {
	Size             Size
	Class            LayoutClass
	RightRailVisible bool
}

// Model is the Bubble Tea model for the static shell.
type Model struct {
	state        ViewState
	size         Size
	layout       LayoutState
	submitPrompt PromptSubmitFunc
	routeCommand CommandRouteFunc
	commandChord bool
	quitting     bool
}

// NewModel creates the default static shell model.
func NewModel() Model {
	return NewModelWithSize(Size{Width: 80, Height: 24})
}

// NewModelWithSize creates a static shell model with deterministic dimensions.
func NewModelWithSize(size Size) Model {
	return NewModelWithSizeAndPromptSubmit(size, nil)
}

// NewModelWithSizeAndPromptSubmit creates a shell model with a prompt submit route.
func NewModelWithSizeAndPromptSubmit(size Size, submitPrompt PromptSubmitFunc) Model {
	return NewModelWithSizePromptSubmitAndCommandRoute(size, submitPrompt, nil)
}

// NewModelWithSizePromptSubmitAndCommandRoute creates a shell model with prompt and command routes.
func NewModelWithSizePromptSubmitAndCommandRoute(size Size, submitPrompt PromptSubmitFunc, routeCommand CommandRouteFunc) Model {
	size = normalizeSize(size)
	return Model{
		state:        IdleEmptyState(),
		size:         size,
		layout:       layoutForSize(size),
		submitPrompt: submitPrompt,
		routeCommand: routeCommand,
	}
}

// NewProgram constructs the Bubble Tea program for the static shell.
func NewProgram(input io.Reader, output io.Writer) *tea.Program {
	return NewProgramWithPromptSubmit(input, output, nil)
}

// NewProgramWithPromptSubmit constructs the Bubble Tea program with app prompt routing.
func NewProgramWithPromptSubmit(input io.Reader, output io.Writer, submitPrompt PromptSubmitFunc) *tea.Program {
	options := []tea.ProgramOption{tea.WithAltScreen()}
	if input != nil {
		options = append(options, tea.WithInput(input))
	}
	if output != nil {
		options = append(options, tea.WithOutput(output))
	}
	return tea.NewProgram(NewModelWithSizeAndPromptSubmit(Size{Width: 80, Height: 24}, submitPrompt), options...)
}

// Init has no startup effect for the static shell.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles presentation-only terminal messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.size = normalizeSize(Size{Width: msg.Width, Height: msg.Height})
		m.layout = layoutForSize(m.size)
	case tea.KeyMsg:
		if m.commandChord {
			m.commandChord = false
			return m, m.routeShortcut(msg)
		}
		if msg.Type == tea.KeyCtrlX {
			m.commandChord = true
			return m, nil
		}
		if m.state.PromptInput == "" && msg.String() == "q" {
			m.quitting = true
			return m, tea.Quit
		}
		return m, m.handlePromptKey(msg)
	}
	return m, nil
}

// View renders the static shell with ANSI styling.
func (m Model) View() string {
	return RenderANSI(m.state, m.size)
}

// Quitting reports whether the single M2 quit path was selected.
func (m Model) Quitting() bool {
	return m.quitting
}

// PromptInput reports the current prompt input view state.
func (m Model) PromptInput() string {
	return m.state.PromptInput
}

// Layout reports the current presentation-only responsive layout state.
func (m Model) Layout() LayoutState {
	return m.layout
}

func (m *Model) handlePromptKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyRunes:
		m.state.PromptInput += string(msg.Runes)
	case tea.KeySpace:
		m.state.PromptInput += " "
	case tea.KeyBackspace, tea.KeyCtrlH:
		m.state.PromptInput = dropLastRune(m.state.PromptInput)
	case tea.KeyEnter:
		if m.state.PromptInput == "" || strings.TrimSpace(m.state.PromptInput) == "" {
			return nil
		}
		text := m.state.PromptInput
		m.state.PromptInput = ""
		if recommendation, ok := policy.RecommendSlashCommand(text); ok {
			return m.routeRecommendation(recommendation)
		}
		if m.submitPrompt != nil {
			turn := m.submitPrompt(text)
			if turn.UserText != "" || turn.AssistantText != "" {
				m.state.Transcript = append(m.state.Transcript, turn)
			}
		}
	}
	return nil
}

func (m *Model) routeShortcut(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	if msg.Type == tea.KeyRunes {
		key = string(msg.Runes)
	}
	recommendation, ok := policy.RecommendShortcut("ctrl+x", key)
	if !ok {
		return nil
	}
	return m.routeRecommendation(recommendation)
}

func (m *Model) routeRecommendation(recommendation policy.CommandRecommendation) tea.Cmd {
	m.showCommandSurface(recommendation)
	if m.routeCommand != nil {
		m.routeCommand(recommendation)
	}
	if recommendation.Route == policy.CommandRouteQuit {
		m.quitting = true
		return tea.Quit
	}
	return nil
}

func (m *Model) showCommandSurface(recommendation policy.CommandRecommendation) {
	m.state.CommandRoute = string(recommendation.Route)
	m.state.RouteSource = "policy.command"
	switch recommendation.Route {
	case policy.CommandRouteStatus:
		m.state.SurfaceTitle = "status"
		m.state.SurfaceLines = []string{
			"Deterministic placeholder status.",
			"stage " + m.state.Phase + " (display-only)",
			"primary model " + m.state.PrimaryModel,
			"utility model " + m.state.UtilityModel,
			"autonomy: " + m.state.Autonomy,
			"git: " + m.state.FooterGit,
			"context: " + m.state.FooterContext,
			"real status sources: deferred",
		}
	case policy.CommandRouteHelp:
		m.state.SurfaceTitle = "help"
		m.state.SurfaceLines = []string{
			"Deterministic placeholder help.",
			"commands:",
			"/status - Show deterministic placeholder status.",
			"/help - Show this deterministic placeholder help.",
			"/quit - Quit Aila.",
			"shortcuts:",
			"ctrl+x s - Show deterministic placeholder status.",
			"ctrl+x q - Quit Aila.",
		}
	}
}

func dropLastRune(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	return string(runes[:len(runes)-1])
}

func normalizeSize(size Size) Size {
	if size.Width <= 0 {
		size.Width = 80
	}
	if size.Height <= 0 {
		size.Height = 24
	}
	return size
}

func layoutForSize(size Size) LayoutState {
	size = normalizeSize(size)
	layout := LayoutState{Size: size, Class: LayoutCompact}
	switch {
	case size.Width >= 140:
		layout.Class = LayoutDesktop
		layout.RightRailVisible = true
	case size.Width >= 120:
		layout.Class = LayoutSpacious
	case size.Width >= 100:
		layout.Class = LayoutStandard
	}
	return layout
}

package tui

import (
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// PromptSubmitFunc routes non-empty submitted prompt text to the application layer.
type PromptSubmitFunc func(text string) TranscriptTurn

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

// Model is the Bubble Tea model for the static shell.
type Model struct {
	state        ViewState
	size         Size
	submitPrompt PromptSubmitFunc
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
	return Model{
		state:        IdleEmptyState(),
		size:         normalizeSize(size),
		submitPrompt: submitPrompt,
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
	case tea.KeyMsg:
		if m.state.PromptInput == "" && msg.String() == "q" {
			m.quitting = true
			return m, tea.Quit
		}
		m.handlePromptKey(msg)
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

func (m *Model) handlePromptKey(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyRunes:
		m.state.PromptInput += string(msg.Runes)
	case tea.KeySpace:
		m.state.PromptInput += " "
	case tea.KeyBackspace, tea.KeyCtrlH:
		m.state.PromptInput = dropLastRune(m.state.PromptInput)
	case tea.KeyEnter:
		if m.state.PromptInput == "" || strings.TrimSpace(m.state.PromptInput) == "" {
			return
		}
		text := m.state.PromptInput
		m.state.PromptInput = ""
		if m.submitPrompt != nil {
			turn := m.submitPrompt(text)
			if turn.UserText != "" || turn.AssistantText != "" {
				m.state.Transcript = append(m.state.Transcript, turn)
			}
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

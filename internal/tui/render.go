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
	ansiGreen           = "\x1b[32m"
	ansiRed             = "\x1b[31m"
	ansiYellow          = "\x1b[33m"
	ansiReset           = "\x1b[0m"
	maxDisplayTextBytes = 240
)

var (
	secretLikeText = regexp.MustCompile(`(?i)(bearer\s+)[^\s,;]+|((?:api[_-]?key|token|password|secret)\s*[:=]\s*)[^\s,;]+`)
	pathLikeText   = regexp.MustCompile(`(?i)(~[^\s,;]*|\$\{?(?:HOME|XDG_[A-Z0-9_]+)\}?[^\s,;]*|/(?:[^\s,;/]+/)+[^\s,;]+|[^\s,;]*(?:\x2eaila|\x2eagentera|\x2econfig|project\.toml|artifacts/|indexes/)[^\s,;]*|[a-z]:\\[^\s,;]+|\\\\[^\s,;]+)`)
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
	Read               *ReadView
	Search             *SearchView
	Approval           *ApprovalProposalView
	ApprovalDecision   *ApprovalDecisionView
	Command            *CommandView
	Utility            *UtilityView
	Compact            *CompactView
	Context            *ContextView
	Fetch              *FetchView
	Mutation           *MutationView
	Recovery           *RecoveryView
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
	RunMemory          *RunMemoryView
	Session            *SessionView
	ModelSwitch        *ModelSwitchView
	AutonomySwitch     *AutonomySwitchView
	PromptEditor       *PromptEditorView
	FileReference      *FileReferenceView
	PolicyRoute        *PolicyRouteView
	Brief              *BriefView
	Vision             *VisionView
	Plan               *PlanView
	Build              *BuildView
	Audit              *AuditView
	PromptPaste        *PromptPasteView
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
	Diff               *DiffView
	DiffSelected       int
	DiffFocus          bool
	PromptInput        string
	PromptDisplayInput string
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
	Mutation    *HistoryMutationItem
	Undo        *HistoryUndoItem
	Recovery    *HistoryRecoveryItem
}

// HistoryMutationItem is app-injected mutation history metadata.
type HistoryMutationItem struct {
	Name                  string
	Status                string
	CommandSource         string
	RequestID             string
	ApprovalID            string
	ApprovalAction        string
	ChangedPaths          []string
	RequestedPath         string
	ExpectedEffect        string
	PreviousVersion       string
	NewVersion            string
	PreviousExists        bool
	BytesWritten          int
	ReplacementCount      int
	ResolvedPathAvailable bool
	ErrorKind             string
	ErrorMessage          string
	DecisionRunID         string
	DecisionCapability    string
}

// HistoryUndoItem is app-injected descriptive undo metadata.
type HistoryUndoItem struct {
	Available       bool
	Action          string
	Paths           []string
	PreviousVersion string
	NewVersion      string
	Reason          string
}

// HistoryRecoveryItem is app-injected recovery history metadata.
type HistoryRecoveryItem struct {
	Command            string
	Status             string
	TargetEventID      string
	Action             string
	Paths              []string
	PreviousVersion    string
	NewVersion         string
	RedoAvailable      bool
	RedoAction         string
	Reason             string
	ErrorKind          string
	ErrorMessage       string
	DecisionRunID      string
	DecisionCapability string
}

// RunMemoryView is app-injected metadata for a stored non-interactive run.
type RunMemoryView struct {
	Mode           string
	Prompt         string
	Status         string
	InspectedFiles []RunMemoryFileView
	Commands       []RunMemoryCommandView
	ChangedFiles   []RunMemoryChangedFileView
	Mutation       *RunMemoryMutationView
	Blockers       []string
	Caveats        []string
	SourceRefs     []string
	StoredSession  bool
	StoredHistory  bool
}

// RunMemoryFileView records one file inspected by a stored read-only run.
type RunMemoryFileView struct {
	Path      string
	Status    string
	LineStart int
	LineEnd   int
	SourceRef string
}

// RunMemoryCommandView records one fixed check executed by a stored read-only run.
type RunMemoryCommandView struct {
	Command  string
	Status   string
	ExitCode int
	Summary  string
}

// RunMemoryChangedFileView records one file changed by a stored write run.
type RunMemoryChangedFileView struct {
	Path            string
	Status          string
	PreviousVersion string
	NewVersion      string
	BytesWritten    int
	SourceRef       string
}

// RunMemoryMutationView records bounded mutation result data for a stored write run.
type RunMemoryMutationView struct {
	Name           string
	Status         string
	Path           string
	ExpectedEffect string
	BytesWritten   int
	ErrorKind      string
	ErrorMessage   string
	Decision       *DecisionView
}

// SessionView is app-injected session lifecycle presentation data. It is
// display-only; TUI code must never discover, read, write, or delete session files.
type SessionView struct {
	Action       string
	Source       string
	Status       string
	SessionID    string
	MemoryStatus string
	Detail       string
	Items        []SessionItemView
	Selected     int
	Focus        bool
}

// SessionItemView records one app-injected selectable session row.
type SessionItemView struct {
	ID           string
	Status       string
	MemoryStatus string
	Detail       string
}

// ModelSwitchView is app-injected model selection data. It is display-only;
// TUI code must never read config, call providers, or inspect credentials.
type ModelSwitchView struct {
	Target         string
	Source         string
	Status         string
	CurrentPrimary string
	CurrentUtility string
	Detail         string
	Items          []ModelSwitchItemView
	Selected       int
	Focus          bool
}

// ModelSwitchItemView records one app-injected model choice row.
type ModelSwitchItemView struct {
	Label            string
	SourceName       string
	Model            string
	Reasoning        string
	Family           string
	Class            string
	Status           string
	CredentialSource string
	Detail           string
	Current          bool
}

// AutonomySwitchView is app-injected autonomy selection data. It is display-only;
// TUI code must never classify or execute operations itself.
type AutonomySwitchView struct {
	Source   string
	Status   string
	Current  string
	Detail   string
	Items    []AutonomySwitchItemView
	Selected int
	Focus    bool
}

// AutonomySwitchItemView records one app-injected autonomy choice row.
type AutonomySwitchItemView struct {
	Level   string
	Status  string
	Detail  string
	Current bool
}

// DiffView is app-injected read-only diff presentation data. It is display-only;
// TUI code must never run git, read files, or mutate workspace state itself.
type DiffView struct {
	Source       string
	Status       string
	Files        []DiffFileView
	Empty        bool
	ErrorMessage string
}

// DiffFileView records one changed file in a rendered diff.
type DiffFileView struct {
	Path    string
	OldPath string
	Status  string
	Hunks   []DiffHunkView
}

// DiffHunkView records one unified diff hunk.
type DiffHunkView struct {
	Header   string
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Lines    []DiffLineView
}

// DiffLineView records one addition, removal, or context line.
type DiffLineView struct {
	Kind    string
	Text    string
	OldLine int
	NewLine int
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

// PolicyRouteView is app-injected policy routing evidence.
type PolicyRouteView struct {
	Source               string
	Input                string
	Candidate            string
	Confidence           int
	Reason               string
	NeededInput          string
	CurrentPhase         string
	RuntimeStatus        string
	RecommendedSuccessor string
	SuccessorValid       bool
	SuccessorRejected    bool
	SuccessorReason      string
	TransitionClaimed    bool
	Executed             bool
	SourceRefs           []PolicyRouteSourceRefView
	BoundaryRequests     []PolicyRouteBoundaryRequestView
}

// PolicyRouteSourceRefView records one policy routing source reference.
type PolicyRouteSourceRefView struct {
	ID      string
	Kind    string
	Path    string
	Command string
	Excerpt string
}

// PolicyRouteBoundaryRequestView records one inert boundary request descriptor.
type PolicyRouteBoundaryRequestView struct {
	Kind      string
	Operation string
	Target    string
	Reason    string
}

// BriefView is app-injected capability orientation output.
type BriefView struct {
	Source              string
	Capability          string
	Signal              string
	Summary             string
	CurrentPhase        string
	RuntimeStatus       string
	KnownGaps           []string
	SuggestedNextAction string
	TransitionClaimed   bool
	DisplayOnly         bool
	SourceRefs          []BriefSourceRefView
	BoundaryRequests    []BriefBoundaryRequestView
}

// BriefSourceRefView records one source reference supporting brief output.
type BriefSourceRefView struct {
	ID      string
	Kind    string
	Path    string
	Command string
	Excerpt string
}

// BriefBoundaryRequestView records one inert brief boundary descriptor.
type BriefBoundaryRequestView struct {
	Kind      string
	Operation string
	Target    string
	Reason    string
}

// BuildView is app-injected bounded build output.
type BuildView struct {
	Source               string
	Capability           string
	Signal               string
	Summary              string
	RecommendedSuccessor string
	SuccessorValid       bool
	TransitionClaimed    bool
	DisplayOnly          bool
	PlanItem             BuildPlanItemView
	Step                 BuildStepView
	Operation            BuildOperationView
	ChangedPaths         []string
	Blockers             []string
	Caveats              []string
	FinalSummary         string
	ArtifactRefs         []BuildArtifactRefView
	SourceRefs           []BuildSourceRefView
	BoundaryRequests     []BuildBoundaryRequestView
}

// BuildPlanItemView records the app-selected plan item for build.
type BuildPlanItemView struct {
	ID     string
	Text   string
	Status string
}

// BuildStepView records the one bounded step and held result.
type BuildStepView struct {
	ID     string
	Text   string
	Status string
}

// BuildOperationView records command, permission, and mutation evidence for build.
type BuildOperationView struct {
	Name             string
	Status           string
	Path             string
	ExpectedEffect   string
	DecisionSource   string
	DecisionAutonomy string
	DecisionAllowed  bool
	ApprovalRequired bool
	BytesWritten     int
	ErrorKind        string
	ErrorMessage     string
}

// BuildArtifactRefView records one build artifact reference.
type BuildArtifactRefView struct {
	ID   string
	Kind string
	Path string
}

// BuildSourceRefView records one source reference supporting build output.
type BuildSourceRefView struct {
	ID      string
	Kind    string
	Path    string
	Command string
	Excerpt string
}

// BuildBoundaryRequestView records one inert build boundary descriptor.
type BuildBoundaryRequestView struct {
	Kind      string
	Operation string
	Target    string
	Reason    string
}

// VisionView is app-injected goal-shaping output.
type VisionView struct {
	Source               string
	Capability           string
	Signal               string
	Phase                string
	Summary              string
	NorthStar            string
	Principles           []string
	LongTermGoals        []string
	Blockers             []string
	NeededInput          string
	NextAction           string
	ArtifactPath         string
	ArtifactStatus       string
	RecommendedSuccessor string
	SuccessorValid       bool
	SuccessorRejected    bool
	SuccessorReason      string
	TransitionClaimed    bool
	DisplayOnly          bool
	ArtifactRefs         []VisionArtifactRefView
	SourceRefs           []VisionSourceRefView
	BoundaryRequests     []VisionBoundaryRequestView
}

// VisionArtifactRefView records one vision artifact reference.
type VisionArtifactRefView struct {
	ID   string
	Kind string
	Path string
}

// VisionSourceRefView records one source reference supporting vision output.
type VisionSourceRefView struct {
	ID      string
	Kind    string
	Path    string
	Command string
	Excerpt string
}

// VisionBoundaryRequestView records one inert vision boundary descriptor.
type VisionBoundaryRequestView struct {
	Kind      string
	Operation string
	Target    string
	Reason    string
}

// AuditView is app-injected read-only audit output.
type AuditView struct {
	Source               string
	Capability           string
	Signal               string
	Summary              string
	EvidenceState        string
	RecommendedSuccessor string
	SuccessorValid       bool
	SuccessorRejected    bool
	SuccessorReason      string
	TransitionClaimed    bool
	DisplayOnly          bool
	Findings             []AuditFindingView
	NextActions          []string
	Caveats              []string
	ArtifactRefs         []AuditArtifactRefView
	SourceRefs           []AuditSourceRefView
	BoundaryRequests     []AuditBoundaryRequestView
}

// AuditFindingView records one app-injected audit finding.
type AuditFindingView struct {
	ID           string
	Severity     string
	Title        string
	Message      string
	SourceRefIDs []string
	NextActions  []string
}

// AuditArtifactRefView records one audit artifact reference.
type AuditArtifactRefView struct {
	ID   string
	Kind string
	Path string
}

// AuditSourceRefView records one source reference supporting audit output.
type AuditSourceRefView struct {
	ID      string
	Kind    string
	Path    string
	Command string
	Excerpt string
}

// AuditBoundaryRequestView records one inert audit boundary descriptor.
type AuditBoundaryRequestView struct {
	Kind      string
	Operation string
	Target    string
	Reason    string
}

// PlanView is app-injected scoped planning output.
type PlanView struct {
	Source               string
	Capability           string
	Signal               string
	Title                string
	Scope                string
	Summary              string
	ArtifactPath         string
	ArtifactStatus       string
	RecommendedSuccessor string
	SuccessorValid       bool
	TransitionClaimed    bool
	DisplayOnly          bool
	Items                []PlanItemView
	Blockers             []string
	NextAction           string
	ArtifactRefs         []PlanArtifactRefView
	SourceRefs           []PlanSourceRefView
	BoundaryRequests     []PlanBoundaryRequestView
}

// PlanItemView records one app-injected plan item.
type PlanItemView struct {
	ID           string
	Text         string
	Status       string
	Done         bool
	Acceptance   []string
	SourceRefIDs []string
}

// PlanArtifactRefView records one plan artifact reference.
type PlanArtifactRefView struct {
	ID   string
	Kind string
	Path string
}

// PlanSourceRefView records one source reference supporting plan output.
type PlanSourceRefView struct {
	ID      string
	Kind    string
	Path    string
	Command string
	Excerpt string
}

// PlanBoundaryRequestView records one inert plan boundary descriptor.
type PlanBoundaryRequestView struct {
	Kind      string
	Operation string
	Target    string
	Reason    string
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
	items = append(items, policyRouteLines(state.PolicyRoute)...)
	items = append(items, briefLines(state.Brief)...)
	items = append(items, visionLines(state.Vision)...)
	items = append(items, auditLines(state.Audit)...)
	items = append(items, buildLines(state.Build)...)
	items = append(items, planLines(state.Plan)...)
	if state.SurfaceTitle == "agent evidence" {
		items = append(items, diagnosticLines(state.Diagnostics)...)
		items = append(items, chatLines(state.Transcript)...)
		items = append(items, approvalLines(state.Approval)...)
		items = append(items, readLines(state.Read)...)
		items = append(items, searchLines(state.Search)...)
		items = append(items, commandLines(state.Command)...)
		items = append(items, fetchLines(state.Fetch)...)
		items = append(items, recoveryLines(state.Recovery)...)
		items = append(items, mutationLines(state.Mutation)...)
		items = append(items, memoryLines(state)...)
		items = append(items, queueLines(state)...)
		items = append(items, surfaceLines(state.CommandRoute, state.RouteSource, state.SurfaceTitle, state.SurfaceLines)...)
		return items
	}
	if state.SurfaceTitle == "session" {
		items = append(items, diagnosticLines(state.Diagnostics)...)
		items = append(items, surfaceLines(state.CommandRoute, state.RouteSource, state.SurfaceTitle, state.SurfaceLines)...)
		items = append(items, memoryLines(state)...)
		items = append(items, queueLines(state)...)
		items = append(items, chatLines(state.Transcript)...)
		return items
	}
	if state.SurfaceTitle != "" {
		items = append(items, diagnosticLines(state.Diagnostics)...)
	}
	items = append(items, approvalLines(state.Approval)...)
	items = append(items, readLines(state.Read)...)
	items = append(items, searchLines(state.Search)...)
	items = append(items, compactLines(state.Compact)...)
	items = append(items, contextLines(state.Context)...)
	items = append(items, commandLines(state.Command)...)
	items = append(items, utilityLines(state.Utility)...)
	items = append(items, fetchLines(state.Fetch)...)
	items = append(items, recoveryLines(state.Recovery)...)
	items = append(items, mutationLines(state.Mutation)...)
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
	if state.RunMemory != nil {
		run := state.RunMemory
		lines = append(lines,
			"  run mode: "+safeText(run.Mode),
			"  run status: "+safeText(run.Status),
			"  run prompt: "+safeText(run.Prompt),
		)
		for _, file := range run.InspectedFiles {
			lines = append(lines, "  inspected file: "+safeText(file.Path)+" status="+safeText(file.Status)+" source_ref="+safeText(file.SourceRef))
		}
		for _, command := range run.Commands {
			lines = append(lines, "  command run: "+safeText(command.Command)+" status="+safeText(command.Status))
		}
		for _, file := range run.ChangedFiles {
			lines = append(lines, "  changed file: "+safeText(file.Path)+" status="+safeText(file.Status)+" source_ref="+safeText(file.SourceRef))
		}
		if run.Mutation != nil {
			lines = append(lines,
				"  mutation tool: "+safeText(run.Mutation.Name),
				"  mutation status: "+safeText(run.Mutation.Status),
				"  mutation path: "+safeText(run.Mutation.Path),
			)
			if run.Mutation.Decision != nil {
				lines = append(lines,
					"  mutation decision source: "+safeText(run.Mutation.Decision.Source),
					"  mutation decision autonomy: "+safeText(run.Mutation.Decision.Autonomy),
					"  mutation approval required: "+boolLabel(run.Mutation.Decision.ApprovalRequired),
				)
			}
		}
		for _, blocker := range run.Blockers {
			lines = append(lines, "  run blocker: "+safeText(blocker))
		}
		for _, caveat := range run.Caveats {
			lines = append(lines, "  run caveat: "+safeText(caveat))
		}
		for _, sourceRef := range run.SourceRefs {
			lines = append(lines, "  source ref: "+safeText(sourceRef))
		}
	}
	return append(lines, "")
}

func hasMemory(state ViewState) bool {
	return state.MemorySource != "" || state.MemorySessionID != "" || len(state.MemoryBlockers) > 0 || len(state.MemoryConcerns) > 0 || state.RunMemory != nil
}

func sessionSurfaceLines(session SessionView) []string {
	lines := []string{
		"source: " + safeText(defaultString(session.Source, "app.session")),
		"action: " + safeText(session.Action),
		"status: " + safeText(session.Status),
		"session id: " + safeText(defaultString(session.SessionID, "current")),
		"memory: " + safeText(session.MemoryStatus),
	}
	if session.Detail != "" {
		lines = append(lines, "detail: "+safeText(session.Detail))
	}
	if len(session.Items) > 0 {
		lines = append(lines, "sessions:")
		selected := clampSessionSelection(session)
		for index, item := range session.Items {
			marker := " "
			if index == selected {
				marker = ">"
			}
			line := marker + " " + safeText(item.ID) + " status=" + safeText(item.Status) + " memory=" + safeText(item.MemoryStatus)
			if item.Detail != "" {
				line += " detail=" + safeText(item.Detail)
			}
			lines = append(lines, line)
		}
	}
	if session.Focus {
		lines = append(lines, "focus: session")
	}
	return append(lines, "app-owned", "display-only")
}

func modelSwitchSurfaceLines(modelSwitch ModelSwitchView) []string {
	selected := clampModelSwitchSelection(modelSwitch)
	lines := []string{
		"source: " + safeText(defaultString(modelSwitch.Source, "app.model")),
		"target: " + safeText(defaultString(modelSwitch.Target, "primary_model")),
		"status: " + safeText(defaultString(modelSwitch.Status, "ready")),
		"current primary: " + safeText(modelSwitch.CurrentPrimary),
		"current utility: " + safeText(modelSwitch.CurrentUtility),
		fmt.Sprintf("selected: %d", selected+1),
	}
	if modelSwitch.Detail != "" {
		lines = append(lines, "detail: "+safeText(modelSwitch.Detail))
	}
	if len(modelSwitch.Items) > 0 {
		lines = append(lines, "models:")
		for index, item := range modelSwitch.Items {
			marker := " "
			if index == selected {
				marker = ">"
			}
			line := marker + " " + safeText(item.Label) + " provider=" + safeText(item.SourceName) + " model=" + safeText(item.Model) + " family=" + safeText(item.Family) + " class=" + safeText(item.Class) + " status=" + safeText(item.Status) + " credential_source=" + safeText(item.CredentialSource) + " current=" + boolLabel(item.Current)
			if item.Reasoning != "" {
				line += " reasoning=" + safeText(item.Reasoning)
			}
			if item.Detail != "" {
				line += " detail=" + safeText(item.Detail)
			}
			lines = append(lines, line)
		}
	}
	if modelSwitch.Focus {
		lines = append(lines, "focus: model")
	}
	return append(lines, "app-owned", "display-only")
}

func autonomySwitchSurfaceLines(autonomySwitch AutonomySwitchView) []string {
	selected := clampAutonomySwitchSelection(autonomySwitch)
	lines := []string{
		"source: " + safeText(defaultString(autonomySwitch.Source, "app.autonomy")),
		"status: " + safeText(defaultString(autonomySwitch.Status, "ready")),
		"current: " + safeText(autonomySwitch.Current),
		fmt.Sprintf("selected: %d", selected+1),
	}
	if autonomySwitch.Detail != "" {
		lines = append(lines, "detail: "+safeText(autonomySwitch.Detail))
	}
	if len(autonomySwitch.Items) > 0 {
		lines = append(lines, "levels:")
		for index, item := range autonomySwitch.Items {
			marker := " "
			if index == selected {
				marker = ">"
			}
			lines = append(lines, marker+" "+safeText(item.Level)+" status="+safeText(item.Status)+" current="+boolLabel(item.Current)+" detail="+safeText(item.Detail))
		}
	}
	if autonomySwitch.Focus {
		lines = append(lines, "focus: auto")
	}
	return append(lines, "app-owned", "display-only")
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

func approvalLines(approval *ApprovalProposalView) []string {
	if approval == nil {
		return nil
	}
	semantic := semanticApproval(approval)
	lines := []string{
		"  Approval pending:",
		"  proposal id: " + semantic.ID,
		"  operation kind: " + semantic.OperationKind,
		"  target: " + semantic.Target,
		"  risk: " + semantic.RiskSummary,
		"  default action: " + semantic.DefaultAction,
	}
	if semantic.Path != "" {
		lines = append(lines, "  path: "+semantic.Path)
	}
	if len(semantic.Command) > 0 {
		lines = append(lines, "  command: "+strings.Join(semantic.Command, " "))
	}
	if semantic.WorkingDir != "" {
		lines = append(lines, "  working dir: "+semantic.WorkingDir)
	}
	if semantic.ExpectedEffect != "" {
		lines = append(lines, "  expected effect: "+semantic.ExpectedEffect)
	}
	if len(semantic.PreviewLines) > 0 {
		lines = append(lines, "  preview:")
		for _, line := range semantic.PreviewLines {
			lines = append(lines, "    "+line)
		}
	}
	if len(semantic.DiffPreview) > 0 {
		lines = append(lines, "  diff preview:")
		for _, line := range semantic.DiffPreview {
			lines = append(lines, "    "+line)
		}
	}
	lines = append(lines,
		"  choices: a approve | n deny | d defer",
		"  mutation executed: false",
		"",
	)
	return lines
}

func readLines(read *ReadView) []string {
	if read == nil {
		return nil
	}
	semantic := semanticRead(read)
	lines := []string{
		"  Read tool:",
		"  tool: " + semantic.Name,
		"  status: " + semantic.Status,
		"  read-only: " + boolLabel(semantic.ReadOnly),
		"  path: " + semantic.Path,
		"  requested range: " + readRangeLabel(semantic.RequestedRange),
		"  completed: " + boolLabel(semantic.Completed),
	}
	if semantic.EffectiveRange != nil {
		lines = append(lines, "  effective range: "+readRangeLabel(*semantic.EffectiveRange))
	}
	if len(semantic.PreviewLines) > 0 {
		lines = append(lines, "  preview:")
		for _, previewLine := range semantic.PreviewLines {
			lines = append(lines, "  | "+previewLine)
		}
	}
	lines = append(lines,
		"  preview truncated: "+boolLabel(semantic.PreviewTruncated),
		"  line limit hit: "+boolLabel(semantic.LineLimitHit),
	)
	if semantic.TruncationMarker != "" {
		lines = append(lines, "  truncation marker: "+semantic.TruncationMarker)
	}
	if semantic.ErrorKind != "" {
		lines = append(lines, "  error kind: "+semantic.ErrorKind)
	}
	if semantic.ErrorMessage != "" {
		lines = append(lines, "  error message: "+semantic.ErrorMessage)
	}
	lines = appendDecisionLines(lines, semantic.Decision)
	lines = append(lines, "")
	return lines
}

func searchLines(search *SearchView) []string {
	if search == nil {
		return nil
	}
	semantic := semanticSearch(search)
	lines := []string{
		"  Search tool:",
		"  tool: " + semantic.Name,
		"  status: " + semantic.Status,
		"  read-only: " + boolLabel(semantic.ReadOnly),
		"  completed: " + boolLabel(semantic.Completed),
	}
	if semantic.Pattern != "" {
		lines = append(lines, "  pattern: "+semantic.Pattern)
	}
	if semantic.Query != "" {
		lines = append(lines, "  query: "+semantic.Query)
	}
	if semantic.IncludePattern != "" {
		lines = append(lines, "  include: "+semantic.IncludePattern)
	}
	if len(semantic.Matches) > 0 {
		lines = append(lines, "  matches:")
		for _, match := range semantic.Matches {
			line := match.Path
			if match.LineNumber > 0 {
				line = fmt.Sprintf("%s:%d: %s", match.Path, match.LineNumber, match.PreviewText)
			}
			lines = append(lines, "  | "+line)
		}
	}
	if !semantic.Completed {
		lines = append(lines, "")
		return lines
	}
	lines = append(lines,
		fmt.Sprintf("  omitted results: %d", semantic.OmittedResults),
		fmt.Sprintf("  omitted files: %d", semantic.OmittedFiles),
		"  preview truncated: "+boolLabel(semantic.PreviewTruncated),
		"  result limit hit: "+boolLabel(semantic.ResultLimitHit),
	)
	if semantic.TruncationMarkers != "" {
		lines = append(lines, "  truncation marker: "+semantic.TruncationMarkers)
	}
	if semantic.ErrorKind != "" {
		lines = append(lines, "  error kind: "+semantic.ErrorKind)
	}
	if semantic.ErrorMessage != "" {
		lines = append(lines, "  error message: "+semantic.ErrorMessage)
	}
	lines = appendDecisionLines(lines, semantic.Decision)
	lines = append(lines, "")
	return lines
}

func commandLines(command *CommandView) []string {
	if command == nil {
		return nil
	}
	semantic := semanticBash(command)
	lines := []string{
		"  Bash command:",
		"  tool: " + semantic.Name,
		"  status: " + semantic.Status,
		"  read-only: " + boolLabel(semantic.ReadOnly),
		"  command: " + strings.Join(semantic.Argv, " "),
		"  working dir: " + semantic.WorkingDir,
		"  completed: " + boolLabel(semantic.Completed),
	}
	if semantic.CommandFamily != "" {
		lines = append(lines, "  command family: "+semantic.CommandFamily)
	}
	if semantic.ExpectedEffect != "" {
		lines = append(lines, "  expected effect: "+semantic.ExpectedEffect)
	}
	if semantic.Completed {
		lines = append(lines, fmt.Sprintf("  exit code: %d", semantic.ExitCode))
	}
	if len(semantic.StdoutLines) > 0 {
		lines = append(lines, "  stdout:")
		for _, line := range semantic.StdoutLines {
			lines = append(lines, "  | "+line)
		}
	}
	if len(semantic.StderrLines) > 0 {
		lines = append(lines, "  stderr:")
		for _, line := range semantic.StderrLines {
			lines = append(lines, "  ! "+line)
		}
	}
	if semantic.Completed {
		lines = append(lines,
			"  stdout truncated: "+boolLabel(semantic.StdoutTruncated),
			"  stderr truncated: "+boolLabel(semantic.StderrTruncated),
		)
	}
	if semantic.ErrorKind != "" {
		lines = append(lines, "  error kind: "+semantic.ErrorKind)
	}
	if semantic.ErrorMessage != "" {
		lines = append(lines, "  error message: "+semantic.ErrorMessage)
	}
	lines = appendDecisionLines(lines, semantic.Decision)
	lines = append(lines, "")
	return lines
}

func utilityLines(utility *UtilityView) []string {
	if utility == nil {
		return nil
	}
	semantic := semanticUtility(utility)
	lines := []string{
		"  Utility worker:",
		"  source: " + semantic.Source,
		"  status: " + semantic.Status,
		"  job: " + semantic.JobKind + " " + semantic.JobID,
		"  model: " + semantic.Model,
		"  read-only: " + boolLabel(semantic.ReadOnly),
	}
	if semantic.Summary != "" {
		lines = append(lines, "  summary: "+semantic.Summary)
	}
	if semantic.PreparedContext != nil {
		line := "  prepared context: " + semantic.PreparedContext.Summary
		if len(semantic.PreparedContext.EvidenceRefIDs) > 0 {
			line += " refs=" + strings.Join(semantic.PreparedContext.EvidenceRefIDs, ", ")
		}
		lines = append(lines, line, "  prepared context non-authoritative: "+boolLabel(semantic.PreparedContext.NonAuthoritative))
		for _, caveat := range semantic.PreparedContext.Caveats {
			lines = append(lines, "  prepared caveat: "+caveat)
		}
	}
	if semantic.StaleContext != nil {
		if semantic.StaleContext.Status != "" {
			lines = append(lines, "  stale context: "+semantic.StaleContext.Status)
		}
		if semantic.StaleContext.Summary != "" {
			line := "  stale context summary: " + semantic.StaleContext.Summary
			if len(semantic.StaleContext.EvidenceRefIDs) > 0 {
				line += " refs=" + strings.Join(semantic.StaleContext.EvidenceRefIDs, ", ")
			}
			lines = append(lines, line)
		}
		for _, caveat := range semantic.StaleContext.Caveats {
			lines = append(lines, "  stale context caveat: "+caveat)
		}
		if semantic.StaleContext.SuggestedNextAction != "" {
			lines = append(lines, "  suggested next action: "+semantic.StaleContext.SuggestedNextAction)
		}
	}
	if semantic.SummaryRefresh != nil {
		if semantic.SummaryRefresh.Status != "" {
			lines = append(lines, "  summary refresh: "+semantic.SummaryRefresh.Status)
		}
		if semantic.SummaryRefresh.OriginalSummary != "" {
			lines = append(lines, "  original summary: "+semantic.SummaryRefresh.OriginalSummary)
		}
		if semantic.SummaryRefresh.RefreshedSummary != "" {
			line := "  refreshed summary: " + semantic.SummaryRefresh.RefreshedSummary
			if len(semantic.SummaryRefresh.SourceRefIDs) > 0 {
				line += " refs=" + strings.Join(semantic.SummaryRefresh.SourceRefIDs, ", ")
			}
			lines = append(lines, line)
		}
		if len(semantic.SummaryRefresh.SourceRefIDs) > 0 {
			lines = append(lines, "  summary refresh source refs: "+strings.Join(semantic.SummaryRefresh.SourceRefIDs, ", "))
		}
		if semantic.SummaryRefresh.Confidence != "" {
			lines = append(lines, "  summary refresh confidence: "+semantic.SummaryRefresh.Confidence)
		}
		for _, detail := range semantic.SummaryRefresh.ExactDetails {
			lines = append(lines, "  summary refresh detail: "+detail)
		}
		for _, caveat := range semantic.SummaryRefresh.Caveats {
			lines = append(lines, "  summary refresh caveat: "+caveat)
		}
	}
	for _, suggestion := range semantic.Suggestions {
		line := "  suggestion: " + suggestion.Text
		if len(suggestion.EvidenceRefIDs) > 0 {
			line += " refs=" + strings.Join(suggestion.EvidenceRefIDs, ", ")
		}
		lines = append(lines, line)
	}
	for _, ref := range semantic.EvidenceRefs {
		lines = append(lines, "  utility evidence: "+ref.ID+" "+ref.Kind+" "+ref.Source+" "+ref.Detail)
	}
	for _, caveat := range semantic.Caveats {
		lines = append(lines, "  caveat: "+caveat)
	}
	if semantic.DeniedReason != "" {
		lines = append(lines, "  denied: "+semantic.DeniedReason+" "+semantic.DeniedDetail)
	}
	lines = append(lines,
		"  file mutation: "+boolLabel(semantic.Safety.FileMutation),
		"  git"+" mutation: "+boolLabel(semantic.Safety.GitMutation),
		"  artifact mutation: "+boolLabel(semantic.Safety.ProjectArtifactMutation),
		"  permission approval: "+boolLabel(semantic.Safety.ApprovalGrant),
		"  workflow transition: "+boolLabel(semantic.Safety.WorkflowPhaseTransition),
		"  final judgment: "+boolLabel(semantic.Safety.FinalJudgment),
		"  context refresh: "+boolLabel(semantic.Safety.ContextRefresh),
		"  context compaction: "+boolLabel(semantic.Safety.ContextCompaction),
		"  context rewrite: "+boolLabel(semantic.Safety.ContextRewrite),
		"",
	)
	return lines
}

func policyRouteLines(route *PolicyRouteView) []string {
	if route == nil {
		return nil
	}
	semantic := semanticPolicyRoute(route)
	lines := []string{
		"  Policy routing:",
		"  source: " + semantic.Source,
		"  candidate: " + semantic.Candidate,
		fmt.Sprintf("  confidence: %d", semantic.Confidence),
		"  current phase: " + semantic.CurrentPhase,
		"  transition claimed: " + boolLabel(semantic.TransitionClaimed),
		"  executed: " + boolLabel(semantic.Executed),
	}
	if semantic.Input != "" {
		lines = append(lines, "  input: "+semantic.Input)
	}
	if semantic.Reason != "" {
		lines = append(lines, "  reason: "+semantic.Reason)
	}
	if semantic.RuntimeStatus != "" {
		lines = append(lines, "  runtime status: "+semantic.RuntimeStatus)
	}
	if semantic.NeededInput != "" {
		lines = append(lines, "  needed input: "+semantic.NeededInput)
	}
	if semantic.RecommendedSuccessor != "" {
		lines = append(lines, "  recommended successor: "+semantic.RecommendedSuccessor)
	}
	if semantic.SuccessorValid || semantic.SuccessorRejected || semantic.SuccessorReason != "" {
		lines = append(lines,
			"  successor valid: "+boolLabel(semantic.SuccessorValid),
			"  successor rejected: "+boolLabel(semantic.SuccessorRejected),
		)
		if semantic.SuccessorReason != "" {
			lines = append(lines, "  successor reason: "+semantic.SuccessorReason)
		}
	}
	for _, request := range semantic.BoundaryRequests {
		line := "  requested effect: " + request.Kind
		if request.Operation != "" {
			line += " operation=" + request.Operation
		}
		if request.Target != "" {
			line += " target=" + request.Target
		}
		if request.Reason != "" {
			line += " reason=" + request.Reason
		}
		lines = append(lines, line)
	}
	for _, ref := range semantic.SourceRefs {
		line := "  source ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			line += " path=" + ref.Path
		}
		if ref.Command != "" {
			line += " command=" + ref.Command
		}
		if ref.Excerpt != "" {
			line += " excerpt=" + ref.Excerpt
		}
		lines = append(lines, line)
	}
	lines = append(lines, "  app-owned", "  display-only", "")
	return lines
}

func briefLines(brief *BriefView) []string {
	if brief == nil {
		return nil
	}
	semantic := semanticBrief(brief)
	lines := []string{
		"  Brief:",
		"  source: " + semantic.Source,
		"  capability: " + semantic.Capability,
		"  signal: " + semantic.Signal,
		"  current phase: " + semantic.CurrentPhase,
		"  runtime status: " + semantic.RuntimeStatus,
		"  transition claimed: " + boolLabel(semantic.TransitionClaimed),
		"  display-only: " + boolLabel(semantic.DisplayOnly),
	}
	if semantic.Summary != "" {
		lines = append(lines, "  summary: "+semantic.Summary)
	}
	for _, gap := range semantic.KnownGaps {
		lines = append(lines, "  known gap: "+gap)
	}
	if semantic.SuggestedNextAction != "" {
		lines = append(lines, "  suggested next action: "+semantic.SuggestedNextAction)
	}
	for _, request := range semantic.BoundaryRequests {
		line := "  requested boundary: " + request.Kind
		if request.Operation != "" {
			line += " operation=" + request.Operation
		}
		if request.Target != "" {
			line += " target=" + request.Target
		}
		if request.Reason != "" {
			line += " reason=" + request.Reason
		}
		lines = append(lines, line)
	}
	for _, ref := range semantic.SourceRefs {
		line := "  source ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			line += " path=" + ref.Path
		}
		if ref.Command != "" {
			line += " command=" + ref.Command
		}
		if ref.Excerpt != "" {
			line += " excerpt=" + ref.Excerpt
		}
		lines = append(lines, line)
	}
	lines = append(lines, "  app-owned", "  display-only", "")
	return lines
}

func planLines(plan *PlanView) []string {
	if plan == nil {
		return nil
	}
	semantic := semanticPlan(plan)
	lines := []string{
		"  Plan:",
		"  source: " + semantic.Source,
		"  capability: " + semantic.Capability,
		"  signal: " + semantic.Signal,
		"  title: " + semantic.Title,
		"  scope: " + semantic.Scope,
		"  artifact: " + semantic.ArtifactPath,
		"  artifact status: " + semantic.ArtifactStatus,
		"  recommended successor: " + semantic.RecommendedSuccessor,
		"  successor valid: " + boolLabel(semantic.SuccessorValid),
		"  transition claimed: " + boolLabel(semantic.TransitionClaimed),
		"  display-only: " + boolLabel(semantic.DisplayOnly),
	}
	if semantic.Summary != "" {
		lines = append(lines, "  summary: "+semantic.Summary)
	}
	for _, item := range semantic.Items {
		lines = append(lines, "  item: "+item.ID+" status="+item.Status+" done="+boolLabel(item.Done)+" text="+item.Text)
		for _, acceptance := range item.Acceptance {
			lines = append(lines, "  acceptance: "+item.ID+" "+acceptance)
		}
		if len(item.SourceRefIDs) > 0 {
			lines = append(lines, "  item source refs: "+item.ID+" "+strings.Join(item.SourceRefIDs, ","))
		}
	}
	for _, blocker := range semantic.Blockers {
		lines = append(lines, "  blocker: "+blocker)
	}
	if semantic.NextAction != "" {
		lines = append(lines, "  next action: "+semantic.NextAction)
	}
	for _, ref := range semantic.ArtifactRefs {
		line := "  artifact ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			line += " path=" + ref.Path
		}
		lines = append(lines, line)
	}
	for _, request := range semantic.BoundaryRequests {
		line := "  requested boundary: " + request.Kind
		if request.Operation != "" {
			line += " operation=" + request.Operation
		}
		if request.Target != "" {
			line += " target=" + request.Target
		}
		if request.Reason != "" {
			line += " reason=" + request.Reason
		}
		lines = append(lines, line)
	}
	for _, ref := range semantic.SourceRefs {
		line := "  source ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			line += " path=" + ref.Path
		}
		if ref.Command != "" {
			line += " command=" + ref.Command
		}
		if ref.Excerpt != "" {
			line += " excerpt=" + ref.Excerpt
		}
		lines = append(lines, line)
	}
	lines = append(lines, "  app-owned", "  display-only", "")
	return lines
}

func buildLines(build *BuildView) []string {
	if build == nil {
		return nil
	}
	semantic := semanticBuild(build)
	lines := []string{
		"  Build:",
		"  source: " + semantic.Source,
		"  capability: " + semantic.Capability,
		"  signal: " + semantic.Signal,
		"  plan item: " + semantic.PlanItem.ID + " status=" + semantic.PlanItem.Status + " text=" + semantic.PlanItem.Text,
		"  step: " + semantic.Step.ID + " status=" + semantic.Step.Status + " text=" + semantic.Step.Text,
		"  tool: " + semantic.Operation.Name + " status=" + semantic.Operation.Status,
		"  path: " + semantic.Operation.Path,
		"  decision source: " + semantic.Operation.DecisionSource,
		"  decision autonomy: " + semantic.Operation.DecisionAutonomy,
		"  decision allowed: " + boolLabel(semantic.Operation.DecisionAllowed),
		"  approval required: " + boolLabel(semantic.Operation.ApprovalRequired),
		"  bytes written: " + fmt.Sprint(semantic.Operation.BytesWritten),
		"  recommended successor: " + semantic.RecommendedSuccessor,
		"  successor valid: " + boolLabel(semantic.SuccessorValid),
		"  transition claimed: " + boolLabel(semantic.TransitionClaimed),
		"  display-only: " + boolLabel(semantic.DisplayOnly),
	}
	if semantic.Summary != "" {
		lines = append(lines, "  summary: "+semantic.Summary)
	}
	if semantic.FinalSummary != "" {
		lines = append(lines, "  final summary: "+semantic.FinalSummary)
	}
	for _, path := range semantic.ChangedPaths {
		lines = append(lines, "  changed path: "+path)
	}
	for _, blocker := range semantic.Blockers {
		lines = append(lines, "  blocker: "+blocker)
	}
	for _, caveat := range semantic.Caveats {
		lines = append(lines, "  caveat: "+caveat)
	}
	if semantic.Operation.ExpectedEffect != "" {
		lines = append(lines, "  expected effect: "+semantic.Operation.ExpectedEffect)
	}
	if semantic.Operation.ErrorKind != "" {
		lines = append(lines, "  error kind: "+semantic.Operation.ErrorKind)
	}
	if semantic.Operation.ErrorMessage != "" {
		lines = append(lines, "  error message: "+semantic.Operation.ErrorMessage)
	}
	for _, ref := range semantic.ArtifactRefs {
		line := "  artifact ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			line += " path=" + ref.Path
		}
		lines = append(lines, line)
	}
	for _, request := range semantic.BoundaryRequests {
		line := "  requested boundary: " + request.Kind
		if request.Operation != "" {
			line += " operation=" + request.Operation
		}
		if request.Target != "" {
			line += " target=" + request.Target
		}
		if request.Reason != "" {
			line += " reason=" + request.Reason
		}
		lines = append(lines, line)
	}
	for _, ref := range semantic.SourceRefs {
		line := "  source ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Excerpt != "" {
			line += " excerpt=" + ref.Excerpt
		}
		lines = append(lines, line)
	}
	return append(lines, "")
}

func visionLines(vision *VisionView) []string {
	if vision == nil {
		return nil
	}
	semantic := semanticVision(vision)
	lines := []string{
		"  Vision:",
		"  source: " + semantic.Source,
		"  capability: " + semantic.Capability,
		"  signal: " + semantic.Signal,
		"  phase: " + semantic.Phase,
		"  artifact: " + semantic.ArtifactPath,
		"  artifact status: " + semantic.ArtifactStatus,
		"  recommended successor: " + semantic.RecommendedSuccessor,
		"  successor valid: " + boolLabel(semantic.SuccessorValid),
		"  successor rejected: " + boolLabel(semantic.SuccessorRejected),
		"  transition claimed: " + boolLabel(semantic.TransitionClaimed),
		"  display-only: " + boolLabel(semantic.DisplayOnly),
	}
	if semantic.SuccessorReason != "" {
		lines = append(lines, "  successor reason: "+semantic.SuccessorReason)
	}
	if semantic.Summary != "" {
		lines = append(lines, "  summary: "+semantic.Summary)
	}
	if semantic.NeededInput != "" {
		lines = append(lines, "  needed input: "+semantic.NeededInput)
	}
	if semantic.NorthStar != "" {
		lines = append(lines, "  north star: "+semantic.NorthStar)
	}
	for _, principle := range semantic.Principles {
		lines = append(lines, "  principle: "+principle)
	}
	for _, goal := range semantic.LongTermGoals {
		lines = append(lines, "  long-term goal: "+goal)
	}
	for _, blocker := range semantic.Blockers {
		lines = append(lines, "  blocker: "+blocker)
	}
	if semantic.NextAction != "" {
		lines = append(lines, "  next action: "+semantic.NextAction)
	}
	for _, ref := range semantic.ArtifactRefs {
		line := "  artifact ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			line += " path=" + ref.Path
		}
		lines = append(lines, line)
	}
	for _, request := range semantic.BoundaryRequests {
		line := "  requested boundary: " + request.Kind
		if request.Operation != "" {
			line += " operation=" + request.Operation
		}
		if request.Target != "" {
			line += " target=" + request.Target
		}
		if request.Reason != "" {
			line += " reason=" + request.Reason
		}
		lines = append(lines, line)
	}
	for _, ref := range semantic.SourceRefs {
		line := "  source ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			line += " path=" + ref.Path
		}
		if ref.Command != "" {
			line += " command=" + ref.Command
		}
		if ref.Excerpt != "" {
			line += " excerpt=" + ref.Excerpt
		}
		lines = append(lines, line)
	}
	return append(lines, "  app-owned", "  display-only", "")
}

func auditLines(audit *AuditView) []string {
	if audit == nil {
		return nil
	}
	semantic := semanticAudit(audit)
	lines := []string{
		"  Audit:",
		"  source: " + semantic.Source,
		"  capability: " + semantic.Capability,
		"  signal: " + semantic.Signal,
		"  evidence: " + semantic.EvidenceState,
		"  recommended successor: " + semantic.RecommendedSuccessor,
		"  successor valid: " + boolLabel(semantic.SuccessorValid),
		"  successor rejected: " + boolLabel(semantic.SuccessorRejected),
		"  transition claimed: " + boolLabel(semantic.TransitionClaimed),
		"  display-only: " + boolLabel(semantic.DisplayOnly),
	}
	if semantic.SuccessorReason != "" {
		lines = append(lines, "  successor reason: "+semantic.SuccessorReason)
	}
	if semantic.Summary != "" {
		lines = append(lines, "  summary: "+semantic.Summary)
	}
	for _, finding := range semantic.Findings {
		lines = append(lines, "  finding: "+finding.ID+" severity="+finding.Severity+" title="+finding.Title)
		if finding.Message != "" {
			lines = append(lines, "  finding message: "+finding.ID+" "+finding.Message)
		}
		if len(finding.SourceRefIDs) > 0 {
			lines = append(lines, "  finding source refs: "+finding.ID+" "+strings.Join(finding.SourceRefIDs, ","))
		}
		for _, action := range finding.NextActions {
			lines = append(lines, "  finding next action: "+finding.ID+" "+action)
		}
	}
	for _, action := range semantic.NextActions {
		lines = append(lines, "  next action: "+action)
	}
	for _, caveat := range semantic.Caveats {
		lines = append(lines, "  caveat: "+caveat)
	}
	for _, ref := range semantic.ArtifactRefs {
		line := "  artifact ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			line += " path=" + ref.Path
		}
		lines = append(lines, line)
	}
	for _, request := range semantic.BoundaryRequests {
		line := "  requested boundary: " + request.Kind
		if request.Operation != "" {
			line += " operation=" + request.Operation
		}
		if request.Target != "" {
			line += " target=" + request.Target
		}
		if request.Reason != "" {
			line += " reason=" + request.Reason
		}
		lines = append(lines, line)
	}
	for _, ref := range semantic.SourceRefs {
		line := "  source ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			line += " path=" + ref.Path
		}
		if ref.Command != "" {
			line += " command=" + ref.Command
		}
		if ref.Excerpt != "" {
			line += " excerpt=" + ref.Excerpt
		}
		lines = append(lines, line)
	}
	return append(lines, "  app-owned", "  display-only", "")
}

func compactLines(compact *CompactView) []string {
	if compact == nil {
		return nil
	}
	lines := []string{
		"  Compact:",
		"  source: " + safeText(defaultString(compact.Source, "app.compact")),
		"  mode: " + safeText(defaultString(compact.Mode, "manual")),
		"  status: " + safeText(defaultString(compact.Status, "completed")),
	}
	if compact.Summary != "" {
		lines = append(lines, "  summary: "+safeText(compact.Summary))
	}
	if compact.OriginalMeter != "" {
		lines = append(lines, "  original meter: "+safeText(compact.OriginalMeter))
	}
	if compact.Meter != "" {
		lines = append(lines, "  meter: "+safeText(compact.Meter))
	}
	for _, caveat := range compact.Caveats {
		lines = append(lines, "  caveat: "+safeText(caveat))
	}
	for _, ref := range compact.SourceRefs {
		label := safeText(ref.ID) + " " + safeText(ref.Kind)
		if ref.Path != "" {
			label += " " + safeText(ref.Path)
		}
		if ref.Command != "" {
			label += " command=" + safeText(ref.Command)
		}
		if ref.Excerpt != "" {
			label += " excerpt=" + safeText(ref.Excerpt)
		}
		lines = append(lines, "  compact source ref: "+label)
	}
	lines = append(lines, "")
	return lines
}

func contextLines(contextView *ContextView) []string {
	if contextView == nil {
		return nil
	}
	lines := []string{
		"  Context:",
		"  source: " + safeText(defaultString(contextView.Source, "app.context")),
		"  status: " + safeText(defaultString(contextView.Status, "ready")),
		"  meter: " + safeText(contextView.Meter),
	}
	for _, block := range contextView.Blocks {
		lines = append(lines, "  block: "+safeText(block.Kind)+" "+safeText(block.Title))
		if block.Text != "" {
			lines = append(lines, "  | "+safeText(block.Text))
		}
		if len(block.SourceRefIDs) > 0 {
			lines = append(lines, "  block refs: "+strings.Join(safeTextSlice(block.SourceRefIDs), ", "))
		}
	}
	for _, claim := range contextView.Claims {
		lines = append(lines, "  claim: "+safeText(claim.Text))
		if len(claim.SourceRefIDs) > 0 {
			lines = append(lines, "  claim refs: "+strings.Join(safeTextSlice(claim.SourceRefIDs), ", "))
		}
	}
	for _, ref := range contextView.SourceRefs {
		label := safeText(ref.ID) + " " + safeText(ref.Kind)
		if ref.Path != "" {
			label += " " + safeText(ref.Path)
		}
		if ref.Command != "" {
			label += " command=" + safeText(ref.Command)
		}
		if ref.Stream != "" {
			label += " stream=" + safeText(ref.Stream)
		}
		if ref.Excerpt != "" {
			label += " excerpt=" + safeText(ref.Excerpt)
		}
		lines = append(lines, "  source ref: "+label)
	}
	for _, warning := range contextView.Warnings {
		lines = append(lines, "  warning: "+safeText(warning))
	}
	lines = append(lines, "")
	return lines
}

func recoveryLines(recovery *RecoveryView) []string {
	if recovery == nil {
		return nil
	}
	lines := []string{
		"  Recovery result:",
		"  command: " + safeText(recovery.Command),
		"  status: " + safeText(recovery.Status),
		"  action: " + safeText(recovery.Action),
	}
	if recovery.TargetEventID != "" {
		lines = append(lines, "  target event id: "+safeText(recovery.TargetEventID))
	}
	if len(recovery.Paths) > 0 {
		lines = append(lines, "  paths: "+safeText(strings.Join(recovery.Paths, ", ")))
	}
	if recovery.PreviousVersion != "" {
		lines = append(lines, "  previous version: "+safeText(recovery.PreviousVersion))
	}
	if recovery.NewVersion != "" {
		lines = append(lines, "  new version: "+safeText(recovery.NewVersion))
	}
	lines = append(lines, "  redo available: "+boolLabel(recovery.RedoAvailable))
	if recovery.RedoAction != "" {
		lines = append(lines, "  redo action: "+safeText(recovery.RedoAction))
	}
	if recovery.Decision != nil {
		lines = append(lines,
			"  decision source: "+recovery.Decision.Source,
			"  approval required: "+boolLabel(recovery.Decision.ApprovalRequired),
			"  operation: "+recovery.Decision.OperationKind,
			"  autonomy: "+recovery.Decision.Autonomy,
		)
		if recovery.Decision.Name != "" {
			lines = append(lines, "  decision tool: "+recovery.Decision.Name)
		}
	}
	if recovery.Reason != "" {
		lines = append(lines, "  reason: "+safeText(recovery.Reason))
	}
	if recovery.ErrorKind != "" {
		lines = append(lines, "  error kind: "+safeText(recovery.ErrorKind))
	}
	if recovery.ErrorMessage != "" {
		lines = append(lines, "  error message: "+safeText(recovery.ErrorMessage))
	}
	lines = append(lines, "")
	return lines
}

func recoverySurfaceLines(recovery *RecoveryView) []string {
	if recovery == nil {
		return []string{"command: recovery", "status: unsupported", "reason: recovery unavailable"}
	}
	lines := []string{
		"command: " + safeText(recovery.Command),
		"status: " + safeText(recovery.Status),
		"action: " + safeText(recovery.Action),
	}
	if recovery.TargetEventID != "" {
		lines = append(lines, "target event id: "+safeText(recovery.TargetEventID))
	}
	if len(recovery.Paths) > 0 {
		lines = append(lines, "paths: "+safeText(strings.Join(recovery.Paths, ", ")))
	}
	if recovery.PreviousVersion != "" {
		lines = append(lines, "previous version: "+safeText(recovery.PreviousVersion))
	}
	if recovery.NewVersion != "" {
		lines = append(lines, "new version: "+safeText(recovery.NewVersion))
	}
	lines = append(lines, "redo available: "+boolLabel(recovery.RedoAvailable))
	if recovery.RedoAction != "" {
		lines = append(lines, "redo action: "+safeText(recovery.RedoAction))
	}
	if recovery.Reason != "" {
		lines = append(lines, "reason: "+safeText(recovery.Reason))
	}
	if recovery.ErrorKind != "" {
		lines = append(lines, "error kind: "+safeText(recovery.ErrorKind))
	}
	if recovery.ErrorMessage != "" {
		lines = append(lines, "error message: "+safeText(recovery.ErrorMessage))
	}
	if recovery.Decision != nil {
		lines = append(lines,
			"decision source: "+recovery.Decision.Source,
			"approval required: "+boolLabel(recovery.Decision.ApprovalRequired),
			"operation: "+recovery.Decision.OperationKind,
			"autonomy: "+recovery.Decision.Autonomy,
		)
		if recovery.Decision.Name != "" {
			lines = append(lines, "decision tool: "+recovery.Decision.Name)
		}
	}
	return lines
}

func mutationLines(mutation *MutationView) []string {
	if mutation == nil {
		return nil
	}
	semantic := semanticMutation(mutation)
	lines := []string{
		"  Mutation result:",
		"  path: " + semantic.Path,
		"  status: " + semantic.Status,
		"  tool: " + semantic.Name,
		"  completed: " + boolLabel(semantic.Completed),
	}
	if semantic.ErrorKind != "" {
		lines = append(lines, "  error kind: "+semantic.ErrorKind)
	}
	lines = append(lines,
		"  previous exists: "+boolLabel(semantic.PreviousExists),
		fmt.Sprintf("  bytes written: %d", semantic.BytesWritten),
	)
	if semantic.Decision != nil {
		lines = append(lines,
			"  decision source: "+semantic.Decision.Source,
			"  approval required: "+boolLabel(semantic.Decision.ApprovalRequired),
			"  operation: "+semantic.Decision.OperationKind,
			"  autonomy: "+semantic.Decision.Autonomy,
		)
		if semantic.Decision.Name != "" {
			lines = append(lines, "  decision tool: "+semantic.Decision.Name)
		}
	}
	if semantic.ExpectedEffect != "" {
		lines = append(lines, "  expected effect: "+semantic.ExpectedEffect)
	}
	if semantic.PreviousVersion != "" {
		lines = append(lines, "  previous version: "+semantic.PreviousVersion)
	}
	if semantic.NewVersion != "" {
		lines = append(lines, "  new version: "+semantic.NewVersion)
	}
	if semantic.ReplacementCount > 0 {
		lines = append(lines, fmt.Sprintf("  replacements: %d", semantic.ReplacementCount))
	}
	if semantic.ErrorMessage != "" {
		lines = append(lines, "  error message: "+semantic.ErrorMessage)
	}
	lines = append(lines, "")
	return lines
}

func fetchLines(fetch *FetchView) []string {
	if fetch == nil {
		return nil
	}
	semantic := semanticFetch(fetch)
	lines := []string{
		"  Fetch result:",
		"  tool: " + semantic.Name,
		"  status: " + semantic.Status,
		"  read-only: " + boolLabel(semantic.ReadOnly),
		"  url: " + semantic.URL,
		"  method: " + semantic.Method,
		"  completed: " + boolLabel(semantic.Completed),
	}
	if semantic.ExpectedEffect != "" {
		lines = append(lines, "  expected effect: "+semantic.ExpectedEffect)
	}
	if semantic.Completed && semantic.HTTPStatusCode > 0 {
		lines = append(lines, fmt.Sprintf("  remote status: %d", semantic.HTTPStatusCode))
	}
	if semantic.HTTPStatus != "" {
		lines = append(lines, "  remote status text: "+semantic.HTTPStatus)
	}
	if semantic.ContentType != "" {
		lines = append(lines, "  content type: "+semantic.ContentType)
	}
	if len(semantic.PreviewLines) > 0 {
		lines = append(lines, "  preview:")
		for _, line := range semantic.PreviewLines {
			lines = append(lines, "  | "+line)
		}
	}
	if semantic.Completed {
		lines = append(lines, "  preview truncated: "+boolLabel(semantic.PreviewTruncated))
		if semantic.OmittedBytesKnown {
			lines = append(lines, fmt.Sprintf("  omitted bytes: %d", semantic.OmittedBytes))
		}
	}
	if semantic.TruncationMarker != "" {
		lines = append(lines, "  truncation marker: "+semantic.TruncationMarker)
	}
	if semantic.ErrorKind != "" {
		lines = append(lines, "  error kind: "+semantic.ErrorKind)
	}
	if semantic.ErrorMessage != "" {
		lines = append(lines, "  error message: "+semantic.ErrorMessage)
	}
	lines = appendDecisionLines(lines, semantic.Decision)
	lines = append(lines, "")
	return lines
}

func appendDecisionLines(lines []string, decision *SemanticDecision) []string {
	if decision == nil {
		return lines
	}
	lines = append(lines,
		"  decision source: "+decision.Source,
		"  decision: "+decisionLabel(decision.Allowed),
		"  decision automatic: "+boolLabel(decision.Automatic),
		"  approval required: "+boolLabel(decision.ApprovalRequired),
		"  autonomy: "+decision.Autonomy,
		"  operation: "+decision.OperationKind,
	)
	if decision.Name != "" {
		lines = append(lines, "  decision tool: "+decision.Name)
	}
	if decision.Target != "" {
		lines = append(lines, "  decision target: "+decision.Target)
	}
	if len(decision.Command) > 0 {
		lines = append(lines, "  decision command: "+strings.Join(decision.Command, " "))
	}
	if decision.WorkingDir != "" {
		lines = append(lines, "  decision working dir: "+decision.WorkingDir)
	}
	if decision.ExpectedEffect != "" {
		lines = append(lines, "  decision expected effect: "+decision.ExpectedEffect)
	}
	lines = append(lines, "  decision reversible: "+boolLabel(decision.Reversible))
	if decision.RunID != "" {
		lines = append(lines, "  decision run id: "+decision.RunID)
	}
	if decision.Capability != "" {
		lines = append(lines, "  decision capability: "+decision.Capability)
	}
	if decision.Reason != "" {
		lines = append(lines, "  decision reason: "+decision.Reason)
	}
	return lines
}

func decisionLabel(allowed bool) string {
	if allowed {
		return "allowed"
	}
	return "denied"
}

func readRangeLabel(lineRange SemanticReadLineRange) string {
	parts := make([]string, 0, 3)
	if lineRange.StartLine > 0 {
		parts = append(parts, fmt.Sprintf("start %d", lineRange.StartLine))
	}
	if lineRange.EndLine > 0 {
		parts = append(parts, fmt.Sprintf("end %d", lineRange.EndLine))
	}
	if lineRange.Limit > 0 {
		parts = append(parts, fmt.Sprintf("limit %d", lineRange.Limit))
	}
	if len(parts) == 0 {
		return "full file"
	}
	return strings.Join(parts, " ")
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
		if ansi {
			text = styleContentLine(text)
		}
		lines = append(lines, panelRow(text, width))
	}
	lines = append(lines, panelBottom(width))
	return lines
}

func styleContentLine(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "+ ") || strings.HasPrefix(trimmed, "> + ") {
		return ansiGreen + text + ansiReset
	}
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "> - ") {
		return ansiRed + text + ansiReset
	}
	return text
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
	input := promptLine(promptDisplayText(state))
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
	for _, code := range []string{ansiBold, ansiDim, ansiCyan, ansiGreen, ansiRed, ansiYellow, ansiReset} {
		text = strings.ReplaceAll(text, code, "")
	}
	return text
}

// SemanticSnapshot is the agent-readable meaning of the rendered static shell.
type SemanticSnapshot struct {
	Scenario       string                  `json:"scenario"`
	Screen         SemanticScreen          `json:"screen"`
	Layout         SemanticLayout          `json:"layout"`
	Session        SemanticSession         `json:"session"`
	Memory         *SemanticMemory         `json:"memory,omitempty"`
	SessionView    *SemanticSessionView    `json:"session_view,omitempty"`
	ModelSwitch    *SemanticModelSwitch    `json:"model_switch,omitempty"`
	AutonomySwitch *SemanticAutonomySwitch `json:"autonomy_switch,omitempty"`
	Diagnostics    []SemanticDiagnostic    `json:"diagnostics,omitempty"`
	Interrupt      *SemanticInterrupt      `json:"interrupt,omitempty"`
	Command        *SemanticCommand        `json:"command,omitempty"`
	PolicyRoute    *SemanticPolicyRoute    `json:"policy_route,omitempty"`
	Brief          *SemanticBrief          `json:"brief,omitempty"`
	Vision         *SemanticVision         `json:"vision,omitempty"`
	Plan           *SemanticPlan           `json:"plan,omitempty"`
	Build          *SemanticBuild          `json:"build,omitempty"`
	Audit          *SemanticAudit          `json:"audit,omitempty"`
	History        *SemanticHistory        `json:"history,omitempty"`
	Diff           *SemanticDiff           `json:"diff,omitempty"`
	Read           *SemanticRead           `json:"read_tool,omitempty"`
	Search         *SemanticSearch         `json:"search_tool,omitempty"`
	Bash           *SemanticBash           `json:"bash_tool,omitempty"`
	Utility        *SemanticUtility        `json:"utility,omitempty"`
	Compact        *SemanticCompact        `json:"compact,omitempty"`
	Context        *SemanticContext        `json:"context,omitempty"`
	Fetch          *SemanticFetch          `json:"fetch_tool,omitempty"`
	Mutation       *SemanticMutation       `json:"mutation_tool,omitempty"`
	Recovery       *SemanticRecovery       `json:"recovery,omitempty"`
	Approval       *SemanticApproval       `json:"approval,omitempty"`
	Regions        []SemanticRegion        `json:"regions"`
	Actions        []SemanticAction        `json:"actions"`
}

// SemanticApproval describes app-injected risky-operation review state.
type SemanticApproval struct {
	ID               string   `json:"id"`
	OperationKind    string   `json:"operation_kind"`
	Target           string   `json:"target"`
	RiskSummary      string   `json:"risk_summary"`
	PreviewLines     []string `json:"preview_lines,omitempty"`
	DefaultAction    string   `json:"default_action"`
	Path             string   `json:"path,omitempty"`
	Command          []string `json:"command,omitempty"`
	WorkingDir       string   `json:"working_dir,omitempty"`
	ExpectedEffect   string   `json:"expected_effect,omitempty"`
	DiffPreview      []string `json:"diff_preview,omitempty"`
	Reversible       bool     `json:"reversible"`
	RunID            string   `json:"run_id,omitempty"`
	Capability       string   `json:"capability,omitempty"`
	MutationExecuted bool     `json:"mutation_executed"`
}

// SemanticRead describes injected read-only state for snapshots.
type SemanticRead struct {
	Name             string                 `json:"tool_name"`
	Status           string                 `json:"status"`
	ReadOnly         bool                   `json:"read_only"`
	Path             string                 `json:"path"`
	RequestedRange   SemanticReadLineRange  `json:"requested_range"`
	EffectiveRange   *SemanticReadLineRange `json:"effective_range,omitempty"`
	PreviewLines     []string               `json:"preview_lines,omitempty"`
	PreviewTruncated bool                   `json:"preview_truncated"`
	LineLimitHit     bool                   `json:"line_limit_hit"`
	TruncationMarker string                 `json:"truncation_marker,omitempty"`
	ErrorKind        string                 `json:"error_kind,omitempty"`
	ErrorMessage     string                 `json:"error_message,omitempty"`
	Decision         *SemanticDecision      `json:"decision,omitempty"`
	Completed        bool                   `json:"completed"`
}

// SemanticReadLineRange records machine-readable 1-based line references.
type SemanticReadLineRange struct {
	StartLine int `json:"start_line,omitempty"`
	EndLine   int `json:"end_line,omitempty"`
	Limit     int `json:"limit,omitempty"`
}

// SemanticSearch describes injected read-only find/grep state for snapshots.
type SemanticSearch struct {
	Name              string                `json:"tool_name"`
	Status            string                `json:"status"`
	ReadOnly          bool                  `json:"read_only"`
	Pattern           string                `json:"pattern,omitempty"`
	Query             string                `json:"query,omitempty"`
	Regex             bool                  `json:"regex,omitempty"`
	IncludePattern    string                `json:"include_pattern,omitempty"`
	Matches           []SemanticSearchMatch `json:"matches,omitempty"`
	OmittedResults    int                   `json:"omitted_results,omitempty"`
	OmittedFiles      int                   `json:"omitted_files,omitempty"`
	PreviewTruncated  bool                  `json:"preview_truncated"`
	ResultLimitHit    bool                  `json:"result_limit_hit"`
	TruncationMarkers string                `json:"truncation_markers,omitempty"`
	ErrorKind         string                `json:"error_kind,omitempty"`
	ErrorMessage      string                `json:"error_message,omitempty"`
	Decision          *SemanticDecision     `json:"decision,omitempty"`
	Completed         bool                  `json:"completed"`
}

// SemanticSearchMatch records one machine-readable find or grep match.
type SemanticSearchMatch struct {
	Path        string `json:"path"`
	LineNumber  int    `json:"line_number,omitempty"`
	PreviewText string `json:"preview_text,omitempty"`
}

// SemanticBash describes injected read-only safe bash state for snapshots.
type SemanticBash struct {
	Name            string            `json:"tool_name"`
	Status          string            `json:"status"`
	ReadOnly        bool              `json:"read_only"`
	Argv            []string          `json:"argv"`
	WorkingDir      string            `json:"working_dir"`
	CommandFamily   string            `json:"command_family,omitempty"`
	ExpectedEffect  string            `json:"expected_effect,omitempty"`
	ExitCode        int               `json:"exit_code"`
	StdoutLines     []string          `json:"stdout_lines,omitempty"`
	StderrLines     []string          `json:"stderr_lines,omitempty"`
	StdoutTruncated bool              `json:"stdout_truncated"`
	StderrTruncated bool              `json:"stderr_truncated"`
	DurationMillis  int64             `json:"duration_millis,omitempty"`
	ErrorKind       string            `json:"error_kind,omitempty"`
	ErrorMessage    string            `json:"error_message,omitempty"`
	Decision        *SemanticDecision `json:"decision,omitempty"`
	Completed       bool              `json:"completed"`
}

// SemanticUtility describes app-injected idle-only utility worker state.
type SemanticUtility struct {
	Source          string                          `json:"source"`
	Status          string                          `json:"status"`
	JobID           string                          `json:"job_id"`
	JobKind         string                          `json:"job_kind"`
	Model           string                          `json:"model"`
	Summary         string                          `json:"summary,omitempty"`
	PreparedContext *SemanticUtilityPreparedContext `json:"prepared_context,omitempty"`
	StaleContext    *SemanticUtilityStaleContext    `json:"stale_context,omitempty"`
	SummaryRefresh  *SemanticUtilitySummaryRefresh  `json:"summary_refresh,omitempty"`
	Suggestions     []SemanticUtilitySuggestion     `json:"suggestions,omitempty"`
	EvidenceRefs    []SemanticUtilityEvidence       `json:"evidence_refs,omitempty"`
	Caveats         []string                        `json:"caveats,omitempty"`
	DeniedReason    string                          `json:"denied_reason,omitempty"`
	DeniedDetail    string                          `json:"denied_detail,omitempty"`
	ReadOnly        bool                            `json:"read_only"`
	Safety          SemanticUtilitySafety           `json:"safety"`
}

// SemanticUtilityPreparedContext records non-authoritative context prep output.
type SemanticUtilityPreparedContext struct {
	Summary          string   `json:"summary"`
	EvidenceRefIDs   []string `json:"evidence_ref_ids,omitempty"`
	Caveats          []string `json:"caveats,omitempty"`
	NonAuthoritative bool     `json:"non_authoritative"`
}

// SemanticUtilityStaleContext records display-only saved-context freshness output.
type SemanticUtilityStaleContext struct {
	Status              string   `json:"status"`
	Summary             string   `json:"summary,omitempty"`
	EvidenceRefIDs      []string `json:"evidence_ref_ids,omitempty"`
	Caveats             []string `json:"caveats,omitempty"`
	SuggestedNextAction string   `json:"suggested_next_action,omitempty"`
}

// SemanticUtilitySummaryRefresh records display-only refreshed summary output.
type SemanticUtilitySummaryRefresh struct {
	Status           string   `json:"status"`
	OriginalSummary  string   `json:"original_summary,omitempty"`
	RefreshedSummary string   `json:"refreshed_summary,omitempty"`
	SourceRefIDs     []string `json:"source_ref_ids,omitempty"`
	ExactDetails     []string `json:"exact_details,omitempty"`
	Confidence       string   `json:"confidence,omitempty"`
	Caveats          []string `json:"caveats,omitempty"`
}

// SemanticUtilitySuggestion records one utility suggestion.
type SemanticUtilitySuggestion struct {
	Text           string   `json:"text"`
	EvidenceRefIDs []string `json:"evidence_ref_ids,omitempty"`
}

// SemanticUtilityEvidence records evidence backing utility output.
type SemanticUtilityEvidence struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Detail string `json:"detail"`
}

// SemanticUtilitySafety records forbidden utility actions; all should remain false.
type SemanticUtilitySafety struct {
	FileMutation            bool `json:"file_mutation"`
	GitMutation             bool `json:"git_mutation"`
	ProjectArtifactMutation bool `json:"project_artifact_mutation"`
	ApprovalGrant           bool `json:"permission_approval"`
	WorkflowPhaseTransition bool `json:"workflow_phase_transition"`
	FinalJudgment           bool `json:"final_judgment"`
	ContextRefresh          bool `json:"context_refresh"`
	ContextCompaction       bool `json:"context_compaction"`
	ContextRewrite          bool `json:"context_rewrite"`
}

// SemanticFetch describes injected read-only network state for snapshots.
type SemanticFetch struct {
	Name              string            `json:"tool_name"`
	Status            string            `json:"status"`
	ReadOnly          bool              `json:"read_only"`
	URL               string            `json:"url"`
	Method            string            `json:"method"`
	ExpectedEffect    string            `json:"expected_effect,omitempty"`
	HTTPStatusCode    int               `json:"http_status_code,omitempty"`
	HTTPStatus        string            `json:"http_status,omitempty"`
	ContentType       string            `json:"content_type,omitempty"`
	PreviewLines      []string          `json:"preview_lines,omitempty"`
	PreviewTruncated  bool              `json:"preview_truncated"`
	OmittedBytesKnown bool              `json:"omitted_bytes_known"`
	OmittedBytes      int64             `json:"omitted_bytes,omitempty"`
	TruncationMarker  string            `json:"truncation_marker,omitempty"`
	DurationMillis    int64             `json:"duration_millis,omitempty"`
	ErrorKind         string            `json:"error_kind,omitempty"`
	ErrorMessage      string            `json:"error_message,omitempty"`
	Decision          *SemanticDecision `json:"decision,omitempty"`
	Completed         bool              `json:"completed"`
}

// SemanticCompact describes app-injected compaction state.
type SemanticCompact struct {
	Source        string                     `json:"source"`
	Mode          string                     `json:"mode"`
	Status        string                     `json:"status"`
	Summary       string                     `json:"summary,omitempty"`
	Meter         string                     `json:"meter,omitempty"`
	OriginalMeter string                     `json:"original_meter,omitempty"`
	Caveats       []string                   `json:"caveats,omitempty"`
	SourceRefs    []SemanticContextSourceRef `json:"source_refs,omitempty"`
}

// SemanticContext describes app-injected context assembly state.
type SemanticContext struct {
	Source     string                     `json:"source"`
	Status     string                     `json:"status"`
	Meter      string                     `json:"meter"`
	Blocks     []SemanticContextBlock     `json:"blocks,omitempty"`
	Claims     []SemanticContextClaim     `json:"claims,omitempty"`
	SourceRefs []SemanticContextSourceRef `json:"source_refs,omitempty"`
	Warnings   []string                   `json:"warnings,omitempty"`
}

// SemanticContextBlock records one context block and supporting refs.
type SemanticContextBlock struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Text         string   `json:"text"`
	SourceRefIDs []string `json:"source_ref_ids,omitempty"`
}

// SemanticContextClaim records a rendered claim and supporting refs.
type SemanticContextClaim struct {
	Text         string   `json:"text"`
	SourceRefIDs []string `json:"source_ref_ids,omitempty"`
}

// SemanticContextSourceRef records exact source evidence for context.
type SemanticContextSourceRef struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Label     string `json:"label,omitempty"`
	Path      string `json:"path,omitempty"`
	LineStart int    `json:"line_start,omitempty"`
	LineEnd   int    `json:"line_end,omitempty"`
	Command   string `json:"command,omitempty"`
	Stream    string `json:"stream,omitempty"`
	Excerpt   string `json:"excerpt,omitempty"`
}

// SemanticRecovery describes injected undo/redo result state for snapshots.
type SemanticRecovery struct {
	Command         string            `json:"command"`
	Status          string            `json:"status"`
	TargetEventID   string            `json:"target_event_id,omitempty"`
	Action          string            `json:"action"`
	Paths           []string          `json:"paths,omitempty"`
	PreviousVersion string            `json:"previous_version,omitempty"`
	NewVersion      string            `json:"new_version,omitempty"`
	RedoAvailable   bool              `json:"redo_available"`
	RedoAction      string            `json:"redo_action,omitempty"`
	Reason          string            `json:"reason,omitempty"`
	ErrorKind       string            `json:"error_kind,omitempty"`
	ErrorMessage    string            `json:"error_message,omitempty"`
	Decision        *SemanticDecision `json:"decision,omitempty"`
	Completed       bool              `json:"completed"`
}

// SemanticMutation describes injected edit/write state for snapshots.
type SemanticMutation struct {
	Name                  string            `json:"tool_name"`
	Status                string            `json:"status"`
	Path                  string            `json:"path"`
	ExpectedEffect        string            `json:"expected_effect,omitempty"`
	PreviousVersion       string            `json:"previous_version,omitempty"`
	NewVersion            string            `json:"new_version,omitempty"`
	PreviousExists        bool              `json:"previous_exists"`
	BytesWritten          int               `json:"bytes_written"`
	ReplacementCount      int               `json:"replacement_count,omitempty"`
	ResolvedPathAvailable bool              `json:"resolved_path_available"`
	ErrorKind             string            `json:"error_kind,omitempty"`
	ErrorMessage          string            `json:"error_message,omitempty"`
	Decision              *SemanticDecision `json:"decision,omitempty"`
	Completed             bool              `json:"completed"`
}

// SemanticDecision describes app-injected autonomy decision evidence.
type SemanticDecision struct {
	Autonomy         string   `json:"autonomy"`
	Source           string   `json:"source"`
	Allowed          bool     `json:"allowed"`
	Automatic        bool     `json:"automatic"`
	ApprovalRequired bool     `json:"approval_required"`
	Reason           string   `json:"reason,omitempty"`
	OperationKind    string   `json:"operation_kind"`
	Name             string   `json:"tool,omitempty"`
	Target           string   `json:"target,omitempty"`
	Command          []string `json:"command,omitempty"`
	WorkingDir       string   `json:"working_dir,omitempty"`
	ExpectedEffect   string   `json:"expected_effect,omitempty"`
	Reversible       bool     `json:"reversible"`
	RunID            string   `json:"run_id,omitempty"`
	Capability       string   `json:"capability,omitempty"`
}

// SemanticMemory describes app-injected resumed current-session memory.
type SemanticMemory struct {
	Source          string             `json:"source"`
	SessionID       string             `json:"session_id"`
	TranscriptTurns int                `json:"transcript_turns"`
	QueuedCount     int                `json:"queued_count"`
	Blockers        []string           `json:"blockers,omitempty"`
	Concerns        []string           `json:"concerns,omitempty"`
	Diagnostics     int                `json:"diagnostics"`
	Run             *SemanticRunMemory `json:"run,omitempty"`
}

// SemanticRunMemory describes a stored non-interactive run.
type SemanticRunMemory struct {
	Mode           string                   `json:"mode"`
	Prompt         string                   `json:"prompt"`
	Status         string                   `json:"status"`
	InspectedFiles []SemanticRunMemoryFile  `json:"inspected_files,omitempty"`
	CommandsRun    []SemanticRunCommand     `json:"commands_run,omitempty"`
	ChangedFiles   []SemanticRunChangedFile `json:"changed_files,omitempty"`
	Mutation       *SemanticRunMutation     `json:"mutation,omitempty"`
	Blockers       []string                 `json:"blockers,omitempty"`
	Caveats        []string                 `json:"caveats,omitempty"`
	SourceRefs     []string                 `json:"source_refs,omitempty"`
	StoredSession  bool                     `json:"stored_session"`
	StoredHistory  bool                     `json:"stored_history"`
}

// SemanticRunMemoryFile records one inspected file in run memory.
type SemanticRunMemoryFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	LineStart int    `json:"line_start,omitempty"`
	LineEnd   int    `json:"line_end,omitempty"`
	SourceRef string `json:"source_ref,omitempty"`
}

// SemanticRunCommand records one command/check in run memory.
type SemanticRunCommand struct {
	Command  string `json:"command"`
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
	Summary  string `json:"summary,omitempty"`
}

// SemanticRunChangedFile records one changed file in run memory.
type SemanticRunChangedFile struct {
	Path            string `json:"path"`
	Status          string `json:"status"`
	PreviousVersion string `json:"previous_version,omitempty"`
	NewVersion      string `json:"new_version,omitempty"`
	BytesWritten    int    `json:"bytes_written,omitempty"`
	SourceRef       string `json:"source_ref,omitempty"`
}

// SemanticRunMutation records mutation result data for a write run.
type SemanticRunMutation struct {
	Name           string            `json:"tool_name"`
	Status         string            `json:"status"`
	Path           string            `json:"path"`
	ExpectedEffect string            `json:"expected_effect,omitempty"`
	BytesWritten   int               `json:"bytes_written,omitempty"`
	ErrorKind      string            `json:"error_kind,omitempty"`
	ErrorMessage   string            `json:"error_message,omitempty"`
	Decision       *SemanticDecision `json:"decision,omitempty"`
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
	SessionID          string `json:"session_id,omitempty"`
	MemoryStatus       string `json:"memory_status,omitempty"`
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

// SemanticPolicyRoute describes app-injected policy routing evidence.
type SemanticPolicyRoute struct {
	Visible              bool                                 `json:"visible"`
	Source               string                               `json:"source"`
	Input                string                               `json:"input,omitempty"`
	Candidate            string                               `json:"candidate,omitempty"`
	Confidence           int                                  `json:"confidence"`
	Reason               string                               `json:"reason,omitempty"`
	NeededInput          string                               `json:"needed_input,omitempty"`
	CurrentPhase         string                               `json:"current_phase,omitempty"`
	RuntimeStatus        string                               `json:"runtime_status,omitempty"`
	RecommendedSuccessor string                               `json:"recommended_successor,omitempty"`
	SuccessorValid       bool                                 `json:"successor_valid"`
	SuccessorRejected    bool                                 `json:"successor_rejected"`
	SuccessorReason      string                               `json:"successor_reason,omitempty"`
	TransitionClaimed    bool                                 `json:"transition_claimed"`
	Executed             bool                                 `json:"executed"`
	SourceRefs           []SemanticPolicyRouteSourceRef       `json:"source_refs,omitempty"`
	BoundaryRequests     []SemanticPolicyRouteBoundaryRequest `json:"boundary_requests,omitempty"`
}

// SemanticPolicyRouteSourceRef records one policy routing source reference.
type SemanticPolicyRouteSourceRef struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// SemanticPolicyRouteBoundaryRequest records one inert boundary descriptor.
type SemanticPolicyRouteBoundaryRequest struct {
	Kind      string `json:"kind"`
	Operation string `json:"operation,omitempty"`
	Target    string `json:"target,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// SemanticBrief describes app-injected brief capability orientation output.
type SemanticBrief struct {
	Visible             bool                           `json:"visible"`
	Source              string                         `json:"source"`
	Capability          string                         `json:"capability"`
	Signal              string                         `json:"signal"`
	Summary             string                         `json:"summary,omitempty"`
	CurrentPhase        string                         `json:"current_phase"`
	RuntimeStatus       string                         `json:"runtime_status"`
	KnownGaps           []string                       `json:"known_gaps,omitempty"`
	SuggestedNextAction string                         `json:"suggested_next_action,omitempty"`
	TransitionClaimed   bool                           `json:"transition_claimed"`
	DisplayOnly         bool                           `json:"display_only"`
	SourceRefs          []SemanticBriefSourceRef       `json:"source_refs,omitempty"`
	BoundaryRequests    []SemanticBriefBoundaryRequest `json:"boundary_requests,omitempty"`
}

// SemanticBriefSourceRef records one brief source reference.
type SemanticBriefSourceRef struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// SemanticBriefBoundaryRequest records one inert brief boundary descriptor.
type SemanticBriefBoundaryRequest struct {
	Kind      string `json:"kind"`
	Operation string `json:"operation,omitempty"`
	Target    string `json:"target,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// SemanticBuild describes app-injected bounded build output.
type SemanticBuild struct {
	Visible              bool                           `json:"visible"`
	Source               string                         `json:"source"`
	Capability           string                         `json:"capability"`
	Signal               string                         `json:"signal"`
	Summary              string                         `json:"summary,omitempty"`
	RecommendedSuccessor string                         `json:"recommended_successor,omitempty"`
	SuccessorValid       bool                           `json:"successor_valid"`
	TransitionClaimed    bool                           `json:"transition_claimed"`
	DisplayOnly          bool                           `json:"display_only"`
	PlanItem             SemanticBuildPlanItem          `json:"plan_item"`
	Step                 SemanticBuildStep              `json:"step"`
	Operation            SemanticBuildOperation         `json:"tool"`
	ChangedPaths         []string                       `json:"changed_paths,omitempty"`
	Blockers             []string                       `json:"blockers,omitempty"`
	Caveats              []string                       `json:"caveats,omitempty"`
	FinalSummary         string                         `json:"final_summary,omitempty"`
	ArtifactRefs         []SemanticBuildArtifactRef     `json:"artifact_refs,omitempty"`
	SourceRefs           []SemanticBuildSourceRef       `json:"source_refs,omitempty"`
	BoundaryRequests     []SemanticBuildBoundaryRequest `json:"boundary_requests,omitempty"`
}

// SemanticBuildPlanItem records the selected plan item.
type SemanticBuildPlanItem struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status string `json:"status"`
}

// SemanticBuildStep records the one bounded build step.
type SemanticBuildStep struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status string `json:"status"`
}

// SemanticBuildOperation records command, permission, and mutation result evidence.
type SemanticBuildOperation struct {
	Name             string `json:"tool_name"`
	Status           string `json:"status"`
	Path             string `json:"path,omitempty"`
	ExpectedEffect   string `json:"expected_effect,omitempty"`
	DecisionSource   string `json:"decision_source,omitempty"`
	DecisionAutonomy string `json:"decision_autonomy,omitempty"`
	DecisionAllowed  bool   `json:"decision_allowed"`
	ApprovalRequired bool   `json:"approval_required"`
	BytesWritten     int    `json:"bytes_written,omitempty"`
	ErrorKind        string `json:"error_kind,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
}

// SemanticBuildArtifactRef records one build artifact reference.
type SemanticBuildArtifactRef struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Path string `json:"path,omitempty"`
}

// SemanticBuildSourceRef records one build source reference.
type SemanticBuildSourceRef struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// SemanticBuildBoundaryRequest records one inert build boundary descriptor.
type SemanticBuildBoundaryRequest struct {
	Kind      string `json:"kind"`
	Operation string `json:"operation,omitempty"`
	Target    string `json:"target,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// SemanticVision describes app-injected goal-shaping output.
type SemanticVision struct {
	Visible              bool                            `json:"visible"`
	Source               string                          `json:"source"`
	Capability           string                          `json:"capability"`
	Signal               string                          `json:"signal"`
	Phase                string                          `json:"phase"`
	Summary              string                          `json:"summary,omitempty"`
	NorthStar            string                          `json:"north_star,omitempty"`
	Principles           []string                        `json:"principles,omitempty"`
	LongTermGoals        []string                        `json:"long_term_goals,omitempty"`
	Blockers             []string                        `json:"blockers,omitempty"`
	NeededInput          string                          `json:"needed_input,omitempty"`
	NextAction           string                          `json:"next_action,omitempty"`
	ArtifactPath         string                          `json:"artifact_path,omitempty"`
	ArtifactStatus       string                          `json:"artifact_status,omitempty"`
	RecommendedSuccessor string                          `json:"recommended_successor,omitempty"`
	SuccessorValid       bool                            `json:"successor_valid"`
	SuccessorRejected    bool                            `json:"successor_rejected"`
	SuccessorReason      string                          `json:"successor_reason,omitempty"`
	TransitionClaimed    bool                            `json:"transition_claimed"`
	DisplayOnly          bool                            `json:"display_only"`
	ArtifactRefs         []SemanticVisionArtifactRef     `json:"artifact_refs,omitempty"`
	SourceRefs           []SemanticVisionSourceRef       `json:"source_refs,omitempty"`
	BoundaryRequests     []SemanticVisionBoundaryRequest `json:"boundary_requests,omitempty"`
}

// SemanticVisionArtifactRef records one vision artifact reference.
type SemanticVisionArtifactRef struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Path string `json:"path,omitempty"`
}

// SemanticVisionSourceRef records one vision source reference.
type SemanticVisionSourceRef struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// SemanticVisionBoundaryRequest records one inert vision boundary descriptor.
type SemanticVisionBoundaryRequest struct {
	Kind      string `json:"kind"`
	Operation string `json:"operation,omitempty"`
	Target    string `json:"target,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// SemanticAudit describes app-injected read-only audit output.
type SemanticAudit struct {
	Visible              bool                           `json:"visible"`
	Source               string                         `json:"source"`
	Capability           string                         `json:"capability"`
	Signal               string                         `json:"signal"`
	Summary              string                         `json:"summary,omitempty"`
	EvidenceState        string                         `json:"evidence_state"`
	RecommendedSuccessor string                         `json:"recommended_successor,omitempty"`
	SuccessorValid       bool                           `json:"successor_valid"`
	SuccessorRejected    bool                           `json:"successor_rejected"`
	SuccessorReason      string                         `json:"successor_reason,omitempty"`
	TransitionClaimed    bool                           `json:"transition_claimed"`
	DisplayOnly          bool                           `json:"display_only"`
	Findings             []SemanticAuditFinding         `json:"findings,omitempty"`
	NextActions          []string                       `json:"next_actions,omitempty"`
	Caveats              []string                       `json:"caveats,omitempty"`
	ArtifactRefs         []SemanticAuditArtifactRef     `json:"artifact_refs,omitempty"`
	SourceRefs           []SemanticAuditSourceRef       `json:"source_refs,omitempty"`
	BoundaryRequests     []SemanticAuditBoundaryRequest `json:"boundary_requests,omitempty"`
}

// SemanticAuditFinding records one machine-readable audit finding.
type SemanticAuditFinding struct {
	ID           string   `json:"id"`
	Severity     string   `json:"severity"`
	Title        string   `json:"title"`
	Message      string   `json:"message,omitempty"`
	SourceRefIDs []string `json:"source_ref_ids,omitempty"`
	NextActions  []string `json:"next_actions,omitempty"`
}

// SemanticAuditArtifactRef records one audit artifact reference.
type SemanticAuditArtifactRef struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Path string `json:"path,omitempty"`
}

// SemanticAuditSourceRef records one audit source reference.
type SemanticAuditSourceRef struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// SemanticAuditBoundaryRequest records one inert audit boundary descriptor.
type SemanticAuditBoundaryRequest struct {
	Kind      string `json:"kind"`
	Operation string `json:"operation,omitempty"`
	Target    string `json:"target,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// SemanticPlan describes app-injected scoped planning output.
type SemanticPlan struct {
	Visible              bool                          `json:"visible"`
	Source               string                        `json:"source"`
	Capability           string                        `json:"capability"`
	Signal               string                        `json:"signal"`
	Title                string                        `json:"title"`
	Scope                string                        `json:"scope"`
	Summary              string                        `json:"summary,omitempty"`
	ArtifactPath         string                        `json:"artifact_path"`
	ArtifactStatus       string                        `json:"artifact_status"`
	RecommendedSuccessor string                        `json:"recommended_successor,omitempty"`
	SuccessorValid       bool                          `json:"successor_valid"`
	TransitionClaimed    bool                          `json:"transition_claimed"`
	DisplayOnly          bool                          `json:"display_only"`
	Items                []SemanticPlanItem            `json:"items,omitempty"`
	Blockers             []string                      `json:"blockers,omitempty"`
	NextAction           string                        `json:"next_action,omitempty"`
	ArtifactRefs         []SemanticPlanArtifactRef     `json:"artifact_refs,omitempty"`
	SourceRefs           []SemanticPlanSourceRef       `json:"source_refs,omitempty"`
	BoundaryRequests     []SemanticPlanBoundaryRequest `json:"boundary_requests,omitempty"`
}

// SemanticPlanItem records one machine-readable plan item.
type SemanticPlanItem struct {
	ID           string   `json:"id"`
	Text         string   `json:"text"`
	Status       string   `json:"status"`
	Done         bool     `json:"done"`
	Acceptance   []string `json:"acceptance,omitempty"`
	SourceRefIDs []string `json:"source_ref_ids,omitempty"`
}

// SemanticPlanArtifactRef records one plan artifact reference.
type SemanticPlanArtifactRef struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Path string `json:"path,omitempty"`
}

// SemanticPlanSourceRef records one plan source reference.
type SemanticPlanSourceRef struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// SemanticPlanBoundaryRequest records one inert plan boundary descriptor.
type SemanticPlanBoundaryRequest struct {
	Kind      string `json:"kind"`
	Operation string `json:"operation,omitempty"`
	Target    string `json:"target,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// SemanticSessionView describes an app-injected session lifecycle surface.
type SemanticSessionView struct {
	Visible      bool                  `json:"visible"`
	Action       string                `json:"action"`
	Source       string                `json:"source"`
	Status       string                `json:"status"`
	SessionID    string                `json:"session_id"`
	MemoryStatus string                `json:"memory_status"`
	Detail       string                `json:"detail,omitempty"`
	Focus        bool                  `json:"focus"`
	Selected     int                   `json:"selected_index"`
	Items        []SemanticSessionItem `json:"items,omitempty"`
}

// SemanticSessionItem describes one inert app-injected selectable session row.
type SemanticSessionItem struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	MemoryStatus string `json:"memory_status"`
	Detail       string `json:"detail,omitempty"`
	Selected     bool   `json:"selected"`
}

// SemanticModelSwitch describes app-injected model selection state.
type SemanticModelSwitch struct {
	Visible        bool                      `json:"visible"`
	Target         string                    `json:"target"`
	Source         string                    `json:"source"`
	Status         string                    `json:"status"`
	CurrentPrimary string                    `json:"current_primary"`
	CurrentUtility string                    `json:"current_utility"`
	Detail         string                    `json:"detail,omitempty"`
	Focus          bool                      `json:"focus"`
	Selected       int                       `json:"selected_index"`
	SelectedLabel  string                    `json:"selected_label,omitempty"`
	Items          []SemanticModelSwitchItem `json:"items"`
}

// SemanticModelSwitchItem describes one app-injected model choice row.
type SemanticModelSwitchItem struct {
	Label            string `json:"label"`
	SourceName       string `json:"provider"`
	Model            string `json:"model"`
	Reasoning        string `json:"reasoning,omitempty"`
	Family           string `json:"family"`
	Class            string `json:"class"`
	Status           string `json:"status"`
	CredentialSource string `json:"credential_source"`
	Detail           string `json:"detail,omitempty"`
	Current          bool   `json:"current"`
	Selected         bool   `json:"selected"`
}

// SemanticAutonomySwitch describes app-injected autonomy selection state.
type SemanticAutonomySwitch struct {
	Visible       bool                         `json:"visible"`
	Source        string                       `json:"source"`
	Status        string                       `json:"status"`
	Current       string                       `json:"current"`
	Detail        string                       `json:"detail,omitempty"`
	Focus         bool                         `json:"focus"`
	Selected      int                          `json:"selected_index"`
	SelectedLevel string                       `json:"selected_level,omitempty"`
	Items         []SemanticAutonomySwitchItem `json:"items"`
}

// SemanticAutonomySwitchItem describes one app-injected autonomy choice row.
type SemanticAutonomySwitchItem struct {
	Level    string `json:"level"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Current  bool   `json:"current"`
	Selected bool   `json:"selected"`
}

// SemanticHistory describes app-injected read-only history presentation state.
type SemanticHistory struct {
	Visible       bool                  `json:"visible"`
	ReadOnly      bool                  `json:"read_only"`
	UndoEnabled   bool                  `json:"undo_enabled"`
	RedoEnabled   bool                  `json:"redo_enabled"`
	Focus         bool                  `json:"focus"`
	Empty         bool                  `json:"empty"`
	Count         int                   `json:"count"`
	SelectedIndex int                   `json:"selected_index"`
	SelectedID    string                `json:"selected_id,omitempty"`
	Items         []SemanticHistoryItem `json:"items"`
}

// SemanticHistoryItem describes one app-injected history row.
type SemanticHistoryItem struct {
	EventID     string                   `json:"event_id"`
	RunID       string                   `json:"run_id"`
	SessionID   string                   `json:"session_id"`
	Kind        string                   `json:"kind"`
	Source      string                   `json:"source"`
	Provenance  string                   `json:"provenance"`
	DisplayText string                   `json:"display_text"`
	Mutation    *SemanticHistoryMutation `json:"mutation,omitempty"`
	Undo        *SemanticHistoryUndo     `json:"undo,omitempty"`
	Recovery    *SemanticHistoryRecovery `json:"recovery,omitempty"`
	Selected    bool                     `json:"selected"`
}

// SemanticHistoryMutation describes mutation metadata inside history snapshots.
type SemanticHistoryMutation struct {
	Name           string   `json:"tool_name"`
	Status         string   `json:"status"`
	CommandSource  string   `json:"command_source"`
	RequestID      string   `json:"request_id,omitempty"`
	ApprovalID     string   `json:"approval_id,omitempty"`
	ApprovalAction string   `json:"approval_action,omitempty"`
	ChangedPaths   []string `json:"changed_paths"`
	RequestedPath  string   `json:"requested_path,omitempty"`
	ExpectedEffect string   `json:"expected_effect,omitempty"`
	ErrorKind      string   `json:"error_kind,omitempty"`
	ErrorMessage   string   `json:"error_message,omitempty"`
}

// SemanticHistoryUndo describes descriptive undo metadata inside history snapshots.
type SemanticHistoryUndo struct {
	Available       bool     `json:"available"`
	Action          string   `json:"action,omitempty"`
	Paths           []string `json:"paths,omitempty"`
	PreviousVersion string   `json:"previous_version,omitempty"`
	NewVersion      string   `json:"new_version,omitempty"`
	Reason          string   `json:"reason,omitempty"`
}

// SemanticHistoryRecovery describes recovery metadata inside history snapshots.
type SemanticHistoryRecovery struct {
	Command            string   `json:"command"`
	Status             string   `json:"status"`
	TargetEventID      string   `json:"target_event_id,omitempty"`
	Action             string   `json:"action"`
	Paths              []string `json:"paths,omitempty"`
	PreviousVersion    string   `json:"previous_version,omitempty"`
	NewVersion         string   `json:"new_version,omitempty"`
	RedoAvailable      bool     `json:"redo_available"`
	RedoAction         string   `json:"redo_action,omitempty"`
	Reason             string   `json:"reason,omitempty"`
	ErrorKind          string   `json:"error_kind,omitempty"`
	ErrorMessage       string   `json:"error_message,omitempty"`
	DecisionRunID      string   `json:"decision_run_id,omitempty"`
	DecisionCapability string   `json:"decision_capability,omitempty"`
}

// SemanticDiff describes app-injected read-only diff presentation state.
type SemanticDiff struct {
	Visible       bool               `json:"visible"`
	ReadOnly      bool               `json:"read_only"`
	Source        string             `json:"source"`
	Status        string             `json:"status"`
	Focus         bool               `json:"focus"`
	Empty         bool               `json:"empty"`
	ErrorMessage  string             `json:"error_message,omitempty"`
	FileCount     int                `json:"file_count"`
	SelectedIndex int                `json:"selected_index"`
	SelectedLine  string             `json:"selected_line,omitempty"`
	Files         []SemanticDiffFile `json:"files"`
}

// SemanticDiffFile describes one file in the diff view.
type SemanticDiffFile struct {
	Path    string             `json:"path"`
	OldPath string             `json:"old_path,omitempty"`
	Status  string             `json:"status"`
	Hunks   []SemanticDiffHunk `json:"hunks"`
}

// SemanticDiffHunk describes one hunk in the diff view.
type SemanticDiffHunk struct {
	Header   string             `json:"header"`
	OldStart int                `json:"old_start,omitempty"`
	OldLines int                `json:"old_lines,omitempty"`
	NewStart int                `json:"new_start,omitempty"`
	NewLines int                `json:"new_lines,omitempty"`
	Lines    []SemanticDiffLine `json:"lines"`
}

// SemanticDiffLine describes one rendered line in a diff hunk.
type SemanticDiffLine struct {
	Kind    string `json:"kind"`
	Text    string `json:"text"`
	OldLine int    `json:"old_line,omitempty"`
	NewLine int    `json:"new_line,omitempty"`
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
	if state.Approval != nil {
		regions = append(regions, SemanticRegion{Name: "approval", Visible: true, Items: semanticApprovalItems(state.Approval)})
	}
	if state.Read != nil {
		regions = append(regions, SemanticRegion{Name: "read_tool", Visible: true, Items: semanticReadItems(state.Read)})
	}
	if state.Search != nil {
		regions = append(regions, SemanticRegion{Name: "search_tool", Visible: true, Items: semanticSearchItems(state.Search)})
	}
	if state.Command != nil {
		regions = append(regions, SemanticRegion{Name: "bash_tool", Visible: true, Items: semanticBashItems(state.Command)})
	}
	if state.Utility != nil {
		regions = append(regions, SemanticRegion{Name: "utility", Visible: true, Items: semanticUtilityItems(state.Utility)})
	}
	if state.Compact != nil {
		regions = append(regions, SemanticRegion{Name: "compact", Visible: true, Items: semanticCompactItems(state.Compact)})
	}
	if state.Context != nil {
		regions = append(regions, SemanticRegion{Name: "context", Visible: true, Items: semanticContextItems(state.Context)})
	}
	if state.Fetch != nil {
		regions = append(regions, SemanticRegion{Name: "fetch_tool", Visible: true, Items: semanticFetchItems(state.Fetch)})
	}
	if state.Mutation != nil {
		regions = append(regions, SemanticRegion{Name: "mutation_tool", Visible: true, Items: semanticMutationItems(state.Mutation)})
	}
	if state.Recovery != nil {
		regions = append(regions, SemanticRegion{Name: "recovery", Visible: true, Items: semanticRecoveryItems(state.Recovery)})
	}
	if hasInterruptState(state) {
		regions = append(regions, SemanticRegion{Name: "interrupt", Visible: true, Items: semanticInterruptItems(state)})
	}
	if state.QueuedCount > 0 {
		regions = append(regions, SemanticRegion{Name: "queue", Visible: true, Items: semanticQueueItems(state)})
	}
	if state.Session != nil {
		regions = append(regions, SemanticRegion{Name: "session", Visible: true, Items: semanticSessionViewItems(state.Session)})
	}
	if state.ModelSwitch != nil {
		regions = append(regions, SemanticRegion{Name: "model_switch", Visible: true, Items: semanticModelSwitchItems(state.ModelSwitch)})
	}
	if state.AutonomySwitch != nil {
		regions = append(regions, SemanticRegion{Name: "autonomy_switch", Visible: true, Items: semanticAutonomySwitchItems(state.AutonomySwitch)})
	}
	if state.FileReference != nil {
		regions = append(regions, SemanticRegion{Name: "file_reference", Visible: true, Items: semanticFileReferenceItems(state.FileReference)})
	}
	if state.PolicyRoute != nil {
		regions = append(regions, SemanticRegion{Name: "policy_route", Visible: true, Items: semanticPolicyRouteItems(state.PolicyRoute)})
	}
	if state.Brief != nil {
		regions = append(regions, SemanticRegion{Name: "brief", Visible: true, Items: semanticBriefItems(state.Brief)})
	}
	if state.Vision != nil {
		regions = append(regions, SemanticRegion{Name: "vision", Visible: true, Items: semanticVisionItems(state.Vision)})
	}
	if state.Plan != nil {
		regions = append(regions, SemanticRegion{Name: "plan", Visible: true, Items: semanticPlanItems(state.Plan)})
	}
	if state.Build != nil {
		regions = append(regions, SemanticRegion{Name: "build", Visible: true, Items: semanticBuildItems(state.Build)})
	}
	if state.Audit != nil {
		regions = append(regions, SemanticRegion{Name: "audit", Visible: true, Items: semanticAuditItems(state.Audit)})
	}
	if state.SurfaceTitle != "" {
		regions = append(regions, SemanticRegion{Name: "command", Visible: true, Items: semanticSurfaceItems(state.CommandRoute, state.RouteSource, state.SurfaceTitle, state.SurfaceLines)})
	}
	if historyVisible(state) {
		regions = append(regions, SemanticRegion{Name: "history", Visible: true, Items: semanticHistoryRegionItems(state)})
	}
	if diffVisible(state) {
		regions = append(regions, SemanticRegion{Name: "diff", Visible: true, Items: semanticDiffRegionItems(state)})
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
		SemanticRegion{Name: "prompt", Visible: true, Items: semanticPromptItems(state)},
		SemanticRegion{Name: "footer", Visible: true, Items: []string{"git: " + state.FooterGit, "context: " + state.FooterContext, "quit: q"}},
	)
	if layout.RightRailVisible {
		regions = append(regions, SemanticRegion{Name: "right_rail", Visible: true, Items: rightRailSemanticItems(state)})
	}
	actions := []SemanticAction{{Name: "quit", Input: "q"}}
	if state.Approval != nil {
		actions = append(actions, approvalActions(state.Approval)...)
	}
	if state.QueuedCount > 0 {
		actions = append(actions, SemanticAction{
			Name:             "queue_after_current_turn",
			Input:            "enter",
			Default:          true,
			PresentationOnly: true,
			Executed:         false,
		})
	}
	if state.ModelSwitch != nil && state.ModelSwitch.Focus {
		actions = append(actions, switchActions("apply model selection")...)
	}
	if state.AutonomySwitch != nil && state.AutonomySwitch.Focus {
		actions = append(actions, switchActions("apply autonomy selection")...)
	}
	actions = append(actions, fileReferenceActions(state.FileReference)...)
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
			SessionID:          semanticSessionID(state),
			MemoryStatus:       semanticSessionMemoryStatus(state),
		},
		Memory:         semanticMemory(state),
		SessionView:    semanticSessionView(state.Session),
		ModelSwitch:    semanticModelSwitch(state.ModelSwitch),
		AutonomySwitch: semanticAutonomySwitch(state.AutonomySwitch),
		Diagnostics:    semanticDiagnostics(state.Diagnostics),
		Command:        command,
		PolicyRoute:    semanticPolicyRoute(state.PolicyRoute),
		Brief:          semanticBrief(state.Brief),
		Vision:         semanticVision(state.Vision),
		Plan:           semanticPlan(state.Plan),
		Build:          semanticBuild(state.Build),
		Audit:          semanticAudit(state.Audit),
		History:        semanticHistory(state),
		Diff:           semanticDiff(state),
		Read:           semanticRead(state.Read),
		Search:         semanticSearch(state.Search),
		Bash:           semanticBash(state.Command),
		Utility:        semanticUtility(state.Utility),
		Compact:        semanticCompact(state.Compact),
		Context:        semanticContext(state.Context),
		Fetch:          semanticFetch(state.Fetch),
		Mutation:       semanticMutation(state.Mutation),
		Recovery:       semanticRecovery(state.Recovery),
		Approval:       semanticApproval(state.Approval),
		Regions:        regions,
		Actions:        actions,
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
			label := "assistant"
			if turn.AssistantStreaming {
				label = "assistant streaming"
			}
			lines = append(lines, "  "+label+": "+safeText(turn.AssistantText))
			if turn.AssistantStreaming {
				lines = append(lines, "  assistant status: incomplete")
			}
			if turn.AssistantSource != "" || turn.AssistantModel != "" {
				lines = append(lines, "  assistant source: "+safeText(turn.AssistantSource)+" "+safeText(turn.AssistantModel))
			}
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
			if turn.AssistantStreaming {
				items = append(items, "assistant_streaming: true", "assistant_incomplete: true", "assistant: "+safeText(turn.AssistantText))
			} else {
				items = append(items, "assistant: "+safeText(turn.AssistantText))
			}
			if turn.AssistantSource != "" {
				items = append(items, "assistant_source: "+safeText(turn.AssistantSource))
			}
			if turn.AssistantModel != "" {
				items = append(items, "assistant_model: "+safeText(turn.AssistantModel))
			}
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

func semanticSessionID(state ViewState) string {
	if state.Session != nil && state.Session.SessionID != "" {
		return safeText(state.Session.SessionID)
	}
	return ""
}

func semanticSessionMemoryStatus(state ViewState) string {
	if state.Session != nil && state.Session.MemoryStatus != "" {
		return safeText(state.Session.MemoryStatus)
	}
	return ""
}

func semanticSessionView(session *SessionView) *SemanticSessionView {
	if session == nil {
		return nil
	}
	selected := clampSessionSelection(*session)
	items := make([]SemanticSessionItem, 0, len(session.Items))
	for index, item := range session.Items {
		items = append(items, SemanticSessionItem{
			ID:           safeText(item.ID),
			Status:       safeText(item.Status),
			MemoryStatus: safeText(item.MemoryStatus),
			Detail:       safeText(item.Detail),
			Selected:     index == selected,
		})
	}
	return &SemanticSessionView{
		Visible:      true,
		Action:       safeText(session.Action),
		Source:       safeText(defaultString(session.Source, "app.session")),
		Status:       safeText(session.Status),
		SessionID:    safeText(defaultString(session.SessionID, "current")),
		MemoryStatus: safeText(session.MemoryStatus),
		Detail:       safeText(session.Detail),
		Focus:        session.Focus,
		Selected:     selected,
		Items:        items,
	}
}

func semanticSessionViewItems(session *SessionView) []string {
	view := semanticSessionView(session)
	if view == nil {
		return nil
	}
	items := []string{
		"source: " + view.Source,
		"action: " + view.Action,
		"status: " + view.Status,
		"session_id: " + view.SessionID,
		"memory_status: " + view.MemoryStatus,
		"focus: " + boolLabel(view.Focus),
		fmt.Sprintf("selected_index: %d", view.Selected),
	}
	if view.Detail != "" {
		items = append(items, "detail: "+view.Detail)
	}
	for _, item := range view.Items {
		items = append(items, "item: "+item.ID+" status="+item.Status+" memory="+item.MemoryStatus+" selected="+boolLabel(item.Selected))
	}
	return append(items, "app-owned", "display-only")
}

func semanticModelSwitch(modelSwitch *ModelSwitchView) *SemanticModelSwitch {
	if modelSwitch == nil {
		return nil
	}
	selected := clampModelSwitchSelection(*modelSwitch)
	items := make([]SemanticModelSwitchItem, 0, len(modelSwitch.Items))
	selectedLabel := ""
	for index, item := range modelSwitch.Items {
		if index == selected {
			selectedLabel = safeText(item.Label)
		}
		items = append(items, SemanticModelSwitchItem{
			Label:            safeText(item.Label),
			SourceName:       safeText(item.SourceName),
			Model:            safeText(item.Model),
			Reasoning:        safeText(item.Reasoning),
			Family:           safeText(item.Family),
			Class:            safeText(item.Class),
			Status:           safeText(item.Status),
			CredentialSource: safeText(item.CredentialSource),
			Detail:           safeText(item.Detail),
			Current:          item.Current,
			Selected:         index == selected,
		})
	}
	return &SemanticModelSwitch{
		Visible:        true,
		Target:         safeText(modelSwitch.Target),
		Source:         safeText(defaultString(modelSwitch.Source, "app.model")),
		Status:         safeText(defaultString(modelSwitch.Status, "ready")),
		CurrentPrimary: safeText(modelSwitch.CurrentPrimary),
		CurrentUtility: safeText(modelSwitch.CurrentUtility),
		Detail:         safeText(modelSwitch.Detail),
		Focus:          modelSwitch.Focus,
		Selected:       selected,
		SelectedLabel:  selectedLabel,
		Items:          items,
	}
}

func semanticModelSwitchItems(modelSwitch *ModelSwitchView) []string {
	semantic := semanticModelSwitch(modelSwitch)
	if semantic == nil {
		return nil
	}
	items := []string{
		"target: " + semantic.Target,
		"source: " + semantic.Source,
		"status: " + semantic.Status,
		"current_primary: " + semantic.CurrentPrimary,
		"current_utility: " + semantic.CurrentUtility,
		"focus: " + boolLabel(semantic.Focus),
		fmt.Sprintf("selected_index: %d", semantic.Selected),
	}
	if semantic.SelectedLabel != "" {
		items = append(items, "selected_label: "+semantic.SelectedLabel)
	}
	if semantic.Detail != "" {
		items = append(items, "detail: "+semantic.Detail)
	}
	for _, item := range semantic.Items {
		line := "item: " + item.Label + " provider=" + item.SourceName + " model=" + item.Model + " family=" + item.Family + " class=" + item.Class + " status=" + item.Status + " credential_source=" + item.CredentialSource + " current=" + boolLabel(item.Current) + " selected=" + boolLabel(item.Selected)
		if item.Reasoning != "" {
			line += " reasoning=" + item.Reasoning
		}
		if item.Detail != "" {
			line += " detail=" + item.Detail
		}
		items = append(items, line)
	}
	return append(items, "app-owned", "display-only")
}

func semanticAutonomySwitch(autonomySwitch *AutonomySwitchView) *SemanticAutonomySwitch {
	if autonomySwitch == nil {
		return nil
	}
	selected := clampAutonomySwitchSelection(*autonomySwitch)
	items := make([]SemanticAutonomySwitchItem, 0, len(autonomySwitch.Items))
	selectedLevel := ""
	for index, item := range autonomySwitch.Items {
		if index == selected {
			selectedLevel = safeText(item.Level)
		}
		items = append(items, SemanticAutonomySwitchItem{
			Level:    safeText(item.Level),
			Status:   safeText(item.Status),
			Detail:   safeText(item.Detail),
			Current:  item.Current,
			Selected: index == selected,
		})
	}
	return &SemanticAutonomySwitch{
		Visible:       true,
		Source:        safeText(defaultString(autonomySwitch.Source, "app.autonomy")),
		Status:        safeText(defaultString(autonomySwitch.Status, "ready")),
		Current:       safeText(autonomySwitch.Current),
		Detail:        safeText(autonomySwitch.Detail),
		Focus:         autonomySwitch.Focus,
		Selected:      selected,
		SelectedLevel: selectedLevel,
		Items:         items,
	}
}

func semanticAutonomySwitchItems(autonomySwitch *AutonomySwitchView) []string {
	semantic := semanticAutonomySwitch(autonomySwitch)
	if semantic == nil {
		return nil
	}
	items := []string{
		"source: " + semantic.Source,
		"status: " + semantic.Status,
		"current: " + semantic.Current,
		"focus: " + boolLabel(semantic.Focus),
		fmt.Sprintf("selected_index: %d", semantic.Selected),
	}
	if semantic.SelectedLevel != "" {
		items = append(items, "selected_level: "+semantic.SelectedLevel)
	}
	if semantic.Detail != "" {
		items = append(items, "detail: "+semantic.Detail)
	}
	for _, item := range semantic.Items {
		items = append(items, "item: "+item.Level+" status="+item.Status+" current="+boolLabel(item.Current)+" selected="+boolLabel(item.Selected)+" detail="+item.Detail)
	}
	return append(items, "app-owned", "display-only")
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
		Run:             semanticRunMemory(state.RunMemory),
	}
}

func semanticRunMemory(run *RunMemoryView) *SemanticRunMemory {
	if run == nil {
		return nil
	}
	files := make([]SemanticRunMemoryFile, 0, len(run.InspectedFiles))
	for _, file := range run.InspectedFiles {
		files = append(files, SemanticRunMemoryFile{Path: safeText(file.Path), Status: safeText(file.Status), LineStart: file.LineStart, LineEnd: file.LineEnd, SourceRef: safeText(file.SourceRef)})
	}
	commands := make([]SemanticRunCommand, 0, len(run.Commands))
	for _, command := range run.Commands {
		commands = append(commands, SemanticRunCommand{Command: safeText(command.Command), Status: safeText(command.Status), ExitCode: command.ExitCode, Summary: safeText(command.Summary)})
	}
	changed := make([]SemanticRunChangedFile, 0, len(run.ChangedFiles))
	for _, file := range run.ChangedFiles {
		changed = append(changed, SemanticRunChangedFile{Path: safeText(file.Path), Status: safeText(file.Status), PreviousVersion: safeText(file.PreviousVersion), NewVersion: safeText(file.NewVersion), BytesWritten: file.BytesWritten, SourceRef: safeText(file.SourceRef)})
	}
	return &SemanticRunMemory{
		Mode:           safeText(run.Mode),
		Prompt:         safeText(run.Prompt),
		Status:         safeText(run.Status),
		InspectedFiles: files,
		CommandsRun:    commands,
		ChangedFiles:   changed,
		Mutation:       semanticRunMutation(run.Mutation),
		Blockers:       safeTextSlice(run.Blockers),
		Caveats:        safeTextSlice(run.Caveats),
		SourceRefs:     safeTextSlice(run.SourceRefs),
		StoredSession:  run.StoredSession,
		StoredHistory:  run.StoredHistory,
	}
}

func semanticRunMutation(mutation *RunMemoryMutationView) *SemanticRunMutation {
	if mutation == nil {
		return nil
	}
	return &SemanticRunMutation{
		Name:           safeText(mutation.Name),
		Status:         safeText(mutation.Status),
		Path:           safeText(mutation.Path),
		ExpectedEffect: safeText(mutation.ExpectedEffect),
		BytesWritten:   mutation.BytesWritten,
		ErrorKind:      safeText(mutation.ErrorKind),
		ErrorMessage:   safeText(mutation.ErrorMessage),
		Decision:       semanticDecision(mutation.Decision),
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
	if memory.Run != nil {
		items = append(items,
			"run_mode: "+memory.Run.Mode,
			"run_status: "+memory.Run.Status,
			"run_prompt: "+memory.Run.Prompt,
			"stored_session: "+boolLabel(memory.Run.StoredSession),
			"stored_history: "+boolLabel(memory.Run.StoredHistory),
		)
		for _, file := range memory.Run.InspectedFiles {
			items = append(items, "inspected_file: "+file.Path+" status="+file.Status+" source_ref="+file.SourceRef)
		}
		for _, command := range memory.Run.CommandsRun {
			items = append(items, "command_run: "+command.Command+" status="+command.Status)
		}
		for _, file := range memory.Run.ChangedFiles {
			items = append(items, "changed_file: "+file.Path+" status="+file.Status+" source_ref="+file.SourceRef)
		}
		if memory.Run.Mutation != nil {
			items = append(items,
				"mutation_tool: "+memory.Run.Mutation.Name,
				"mutation_status: "+memory.Run.Mutation.Status,
				"mutation_path: "+memory.Run.Mutation.Path,
			)
			if memory.Run.Mutation.Decision != nil {
				items = append(items,
					"mutation_decision_source: "+memory.Run.Mutation.Decision.Source,
					"mutation_decision_autonomy: "+memory.Run.Mutation.Decision.Autonomy,
					"mutation_approval_required: "+boolLabel(memory.Run.Mutation.Decision.ApprovalRequired),
				)
			}
		}
		for _, blocker := range memory.Run.Blockers {
			items = append(items, "run_blocker: "+blocker)
		}
		for _, caveat := range memory.Run.Caveats {
			items = append(items, "run_caveat: "+caveat)
		}
		for _, sourceRef := range memory.Run.SourceRefs {
			items = append(items, "source_ref: "+sourceRef)
		}
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

func semanticApprovalItems(approval *ApprovalProposalView) []string {
	semantic := semanticApproval(approval)
	if semantic == nil {
		return nil
	}
	items := []string{
		"proposal_id: " + semantic.ID,
		"operation_kind: " + semantic.OperationKind,
		"target: " + semantic.Target,
		"risk_summary: " + semantic.RiskSummary,
		"default_action: " + semantic.DefaultAction,
		"mutation_executed: false",
	}
	if semantic.Path != "" {
		items = append(items, "path: "+semantic.Path)
	}
	if len(semantic.Command) > 0 {
		items = append(items, "command: "+strings.Join(semantic.Command, " "))
	}
	if semantic.WorkingDir != "" {
		items = append(items, "working_dir: "+semantic.WorkingDir)
	}
	if semantic.ExpectedEffect != "" {
		items = append(items, "expected_effect: "+semantic.ExpectedEffect)
	}
	for _, line := range semantic.PreviewLines {
		items = append(items, "preview_line: "+line)
	}
	for _, line := range semantic.DiffPreview {
		items = append(items, "diff_preview_line: "+line)
	}
	items = append(items, "choice: approve input=a", "choice: deny input=n", "choice: defer input=d", "app-owned", "display-only")
	return items
}

func semanticApproval(approval *ApprovalProposalView) *SemanticApproval {
	if approval == nil {
		return nil
	}
	defaultAction := safeText(approval.DefaultAction)
	if defaultAction == "" {
		defaultAction = "deny"
	}
	operationKind := safeText(approval.OperationKind)
	if operationKind == "" {
		operationKind = "risky"
	}
	target := safeText(approval.Target)
	if target == "" {
		target = safeText(approval.Path)
	}
	return &SemanticApproval{
		ID:               safeText(approval.ID),
		OperationKind:    operationKind,
		Target:           target,
		RiskSummary:      safeText(approval.RiskSummary),
		PreviewLines:     safePreviewLines(approval.PreviewLines),
		DefaultAction:    defaultAction,
		Path:             safeText(approval.Path),
		Command:          safeTextSlice(approval.Command),
		WorkingDir:       safeText(approval.WorkingDir),
		ExpectedEffect:   safeText(approval.ExpectedEffect),
		DiffPreview:      safePreviewLines(approval.DiffPreview),
		Reversible:       approval.Reversible,
		RunID:            safeText(approval.RunID),
		Capability:       safeText(approval.Capability),
		MutationExecuted: false,
	}
}

func approvalActions(approval *ApprovalProposalView) []SemanticAction {
	defaultAction := approval.DefaultAction
	if defaultAction == "" {
		defaultAction = "deny"
	}
	return []SemanticAction{
		{Name: "approve proposal", Input: "a", Default: defaultAction == "approve", PresentationOnly: true, Executed: false},
		{Name: "deny proposal", Input: "n", Default: defaultAction == "deny", PresentationOnly: true, Executed: false},
		{Name: "defer proposal", Input: "d", Default: defaultAction == "defer", PresentationOnly: true, Executed: false},
	}
}

func switchActions(defaultName string) []SemanticAction {
	return []SemanticAction{
		{Name: "move selection up", Input: "up", PresentationOnly: true, Executed: false},
		{Name: "move selection down", Input: "down", PresentationOnly: true, Executed: false},
		{Name: defaultName, Input: "enter", Default: true, PresentationOnly: true, Executed: false},
		{Name: "release selection focus", Input: "esc", PresentationOnly: true, Executed: false},
	}
}

func semanticReadItems(read *ReadView) []string {
	semantic := semanticRead(read)
	if semantic == nil {
		return nil
	}
	items := []string{
		"tool_name: " + semantic.Name,
		"status: " + semantic.Status,
		"read_only: " + boolLabel(semantic.ReadOnly),
		"path: " + semantic.Path,
		"requested_range: " + readRangeLabel(semantic.RequestedRange),
		"completed: " + boolLabel(semantic.Completed),
	}
	if semantic.EffectiveRange != nil {
		items = append(items, "effective_range: "+readRangeLabel(*semantic.EffectiveRange))
	}
	for _, previewLine := range semantic.PreviewLines {
		items = append(items, "preview_line: "+previewLine)
	}
	items = append(items,
		"preview_truncated: "+boolLabel(semantic.PreviewTruncated),
		"line_limit_hit: "+boolLabel(semantic.LineLimitHit),
	)
	if semantic.TruncationMarker != "" {
		items = append(items, "truncation_marker: "+semantic.TruncationMarker)
	}
	if semantic.ErrorKind != "" {
		items = append(items, "error_kind: "+semantic.ErrorKind)
	}
	if semantic.ErrorMessage != "" {
		items = append(items, "error_message: "+semantic.ErrorMessage)
	}
	items = appendDecisionItems(items, semantic.Decision)
	items = append(items, "app-owned", "display-only")
	return items
}

func semanticRead(read *ReadView) *SemanticRead {
	if read == nil {
		return nil
	}
	status := safeText(read.Status)
	if status == "" {
		status = "running"
	}
	completed := status != "running"
	if read.ErrorKind != "" {
		completed = true
	}
	name := safeText(read.Name)
	if name == "" {
		name = "read"
	}
	semantic := &SemanticRead{
		Name:             name,
		Status:           status,
		ReadOnly:         read.ReadOnly,
		Path:             safeReadTargetPath(read.Path),
		RequestedRange:   semanticReadLineRange(read.RequestedRange),
		PreviewLines:     safePreviewLines(read.PreviewLines),
		PreviewTruncated: read.PreviewTruncated,
		LineLimitHit:     read.LineLimitHit,
		TruncationMarker: safeText(read.TruncationMarker),
		ErrorKind:        safeText(read.ErrorKind),
		ErrorMessage:     safeText(read.ErrorMessage),
		Decision:         semanticDecision(read.Decision),
		Completed:        completed,
	}
	if hasReadRange(read.EffectiveRange) {
		effective := semanticReadLineRange(read.EffectiveRange)
		semantic.EffectiveRange = &effective
	}
	if !semantic.Completed {
		semantic.EffectiveRange = nil
		semantic.PreviewLines = nil
		semantic.PreviewTruncated = false
		semantic.LineLimitHit = false
		semantic.TruncationMarker = ""
		semantic.ErrorKind = ""
		semantic.ErrorMessage = ""
		semantic.Decision = nil
	}
	return semantic
}

func semanticSearchItems(search *SearchView) []string {
	semantic := semanticSearch(search)
	if semantic == nil {
		return nil
	}
	items := []string{
		"tool_name: " + semantic.Name,
		"status: " + semantic.Status,
		"read_only: " + boolLabel(semantic.ReadOnly),
		"completed: " + boolLabel(semantic.Completed),
	}
	if semantic.Pattern != "" {
		items = append(items, "pattern: "+semantic.Pattern)
	}
	if semantic.Query != "" {
		items = append(items, "query: "+semantic.Query)
	}
	if semantic.IncludePattern != "" {
		items = append(items, "include_pattern: "+semantic.IncludePattern)
	}
	for _, match := range semantic.Matches {
		if match.LineNumber > 0 {
			items = append(items, fmt.Sprintf("match: %s:%d: %s", match.Path, match.LineNumber, match.PreviewText))
		} else {
			items = append(items, "match: "+match.Path)
		}
	}
	if !semantic.Completed {
		items = append(items, "app-owned", "display-only")
		return items
	}
	items = append(items,
		fmt.Sprintf("omitted_results: %d", semantic.OmittedResults),
		fmt.Sprintf("omitted_files: %d", semantic.OmittedFiles),
		"preview_truncated: "+boolLabel(semantic.PreviewTruncated),
		"result_limit_hit: "+boolLabel(semantic.ResultLimitHit),
	)
	if semantic.TruncationMarkers != "" {
		items = append(items, "truncation_markers: "+semantic.TruncationMarkers)
	}
	if semantic.ErrorKind != "" {
		items = append(items, "error_kind: "+semantic.ErrorKind)
	}
	if semantic.ErrorMessage != "" {
		items = append(items, "error_message: "+semantic.ErrorMessage)
	}
	items = appendDecisionItems(items, semantic.Decision)
	items = append(items, "app-owned", "display-only")
	return items
}

func semanticSearch(search *SearchView) *SemanticSearch {
	if search == nil {
		return nil
	}
	status := safeText(search.Status)
	if status == "" {
		status = "running"
	}
	completed := status != "running"
	if search.ErrorKind != "" {
		completed = true
	}
	name := safeText(search.Name)
	if name == "" {
		name = "search"
	}
	semantic := &SemanticSearch{
		Name:              name,
		Status:            status,
		ReadOnly:          search.ReadOnly,
		Pattern:           safeSearchTarget(search.Pattern),
		Query:             safeText(search.Query),
		Regex:             search.Regex,
		IncludePattern:    safeSearchTarget(search.IncludePattern),
		Matches:           semanticSearchMatches(search.Matches),
		OmittedResults:    search.OmittedResults,
		OmittedFiles:      search.OmittedFiles,
		PreviewTruncated:  search.PreviewTruncated,
		ResultLimitHit:    search.ResultLimitHit,
		TruncationMarkers: safeText(search.TruncationMarkers),
		ErrorKind:         safeText(search.ErrorKind),
		ErrorMessage:      safeText(search.ErrorMessage),
		Decision:          semanticDecision(search.Decision),
		Completed:         completed,
	}
	if !semantic.Completed {
		semantic.Matches = nil
		semantic.OmittedResults = 0
		semantic.OmittedFiles = 0
		semantic.PreviewTruncated = false
		semantic.ResultLimitHit = false
		semantic.TruncationMarkers = ""
		semantic.ErrorKind = ""
		semantic.ErrorMessage = ""
		semantic.Decision = nil
	}
	return semantic
}

func semanticSearchMatches(matches []SearchMatchView) []SemanticSearchMatch {
	if len(matches) == 0 {
		return nil
	}
	const maxMatches = 12
	limit := len(matches)
	if limit > maxMatches {
		limit = maxMatches
	}
	items := make([]SemanticSearchMatch, 0, limit)
	for _, match := range matches[:limit] {
		items = append(items, SemanticSearchMatch{Path: safeSearchTarget(match.Path), LineNumber: match.LineNumber, PreviewText: safeText(match.PreviewText)})
	}
	return items
}

func semanticBashItems(command *CommandView) []string {
	semantic := semanticBash(command)
	if semantic == nil {
		return nil
	}
	items := []string{
		"tool_name: " + semantic.Name,
		"status: " + semantic.Status,
		"read_only: " + boolLabel(semantic.ReadOnly),
		"command: " + strings.Join(semantic.Argv, " "),
		"working_dir: " + semantic.WorkingDir,
		"completed: " + boolLabel(semantic.Completed),
	}
	if semantic.CommandFamily != "" {
		items = append(items, "command_family: "+semantic.CommandFamily)
	}
	if semantic.ExpectedEffect != "" {
		items = append(items, "expected_effect: "+semantic.ExpectedEffect)
	}
	if semantic.Completed {
		items = append(items, fmt.Sprintf("exit_code: %d", semantic.ExitCode))
	}
	for _, line := range semantic.StdoutLines {
		items = append(items, "stdout_line: "+line)
	}
	for _, line := range semantic.StderrLines {
		items = append(items, "stderr_line: "+line)
	}
	if semantic.Completed {
		items = append(items,
			"stdout_truncated: "+boolLabel(semantic.StdoutTruncated),
			"stderr_truncated: "+boolLabel(semantic.StderrTruncated),
		)
	}
	if semantic.ErrorKind != "" {
		items = append(items, "error_kind: "+semantic.ErrorKind)
	}
	if semantic.ErrorMessage != "" {
		items = append(items, "error_message: "+semantic.ErrorMessage)
	}
	items = appendDecisionItems(items, semantic.Decision)
	items = append(items, "app-owned", "display-only")
	return items
}

func semanticBash(command *CommandView) *SemanticBash {
	if command == nil {
		return nil
	}
	status := safeText(command.Status)
	if status == "" {
		status = "running"
	}
	completed := status == "completed" || status == "failed"
	if command.ErrorKind != "" {
		completed = true
	}
	name := safeText(command.Name)
	if name == "" {
		name = "bash"
	}
	semantic := &SemanticBash{
		Name:            name,
		Status:          status,
		ReadOnly:        command.ReadOnly,
		Argv:            safeCommandArgv(command.Argv),
		WorkingDir:      safeCommandPath(command.WorkingDir),
		CommandFamily:   safeText(command.CommandFamily),
		ExpectedEffect:  safeText(command.ExpectedEffect),
		ExitCode:        command.ExitCode,
		StdoutLines:     safeCommandOutputLines(command.StdoutLines),
		StderrLines:     safeCommandOutputLines(command.StderrLines),
		StdoutTruncated: command.StdoutTruncated,
		StderrTruncated: command.StderrTruncated,
		DurationMillis:  command.DurationMillis,
		ErrorKind:       safeText(command.ErrorKind),
		ErrorMessage:    safeText(command.ErrorMessage),
		Decision:        semanticDecision(command.Decision),
		Completed:       completed,
	}
	if !semantic.Completed {
		semantic.ExitCode = 0
		semantic.StdoutLines = nil
		semantic.StderrLines = nil
		semantic.StdoutTruncated = false
		semantic.StderrTruncated = false
		semantic.DurationMillis = 0
		semantic.ErrorKind = ""
		semantic.ErrorMessage = ""
	}
	return semantic
}

func semanticPolicyRouteItems(route *PolicyRouteView) []string {
	semantic := semanticPolicyRoute(route)
	if semantic == nil {
		return nil
	}
	items := []string{
		"source: " + semantic.Source,
		"candidate: " + semantic.Candidate,
		fmt.Sprintf("confidence: %d", semantic.Confidence),
		"current_phase: " + semantic.CurrentPhase,
		"transition_claimed: " + boolLabel(semantic.TransitionClaimed),
		"executed: " + boolLabel(semantic.Executed),
		"display-only",
	}
	if semantic.Input != "" {
		items = append(items, "input: "+semantic.Input)
	}
	if semantic.Reason != "" {
		items = append(items, "reason: "+semantic.Reason)
	}
	if semantic.RuntimeStatus != "" {
		items = append(items, "runtime_status: "+semantic.RuntimeStatus)
	}
	if semantic.NeededInput != "" {
		items = append(items, "needed_input: "+semantic.NeededInput)
	}
	if semantic.RecommendedSuccessor != "" {
		items = append(items, "recommended_successor: "+semantic.RecommendedSuccessor)
	}
	if semantic.SuccessorValid || semantic.SuccessorRejected || semantic.SuccessorReason != "" {
		items = append(items,
			"successor_valid: "+boolLabel(semantic.SuccessorValid),
			"successor_rejected: "+boolLabel(semantic.SuccessorRejected),
		)
		if semantic.SuccessorReason != "" {
			items = append(items, "successor_reason: "+semantic.SuccessorReason)
		}
	}
	for _, request := range semantic.BoundaryRequests {
		item := "boundary_request: " + request.Kind
		if request.Operation != "" {
			item += " operation=" + request.Operation
		}
		if request.Target != "" {
			item += " target=" + request.Target
		}
		if request.Reason != "" {
			item += " reason=" + request.Reason
		}
		items = append(items, item)
	}
	for _, ref := range semantic.SourceRefs {
		item := "source_ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			item += " path=" + ref.Path
		}
		if ref.Command != "" {
			item += " command=" + ref.Command
		}
		if ref.Excerpt != "" {
			item += " excerpt=" + ref.Excerpt
		}
		items = append(items, item)
	}
	return items
}

func semanticPolicyRoute(route *PolicyRouteView) *SemanticPolicyRoute {
	if route == nil {
		return nil
	}
	refs := make([]SemanticPolicyRouteSourceRef, 0, len(route.SourceRefs))
	for _, ref := range route.SourceRefs {
		refs = append(refs, SemanticPolicyRouteSourceRef{
			ID:      safeText(ref.ID),
			Kind:    safeText(ref.Kind),
			Path:    safeText(ref.Path),
			Command: safeText(ref.Command),
			Excerpt: safeText(ref.Excerpt),
		})
	}
	requests := make([]SemanticPolicyRouteBoundaryRequest, 0, len(route.BoundaryRequests))
	for _, request := range route.BoundaryRequests {
		requests = append(requests, SemanticPolicyRouteBoundaryRequest{
			Kind:      safeText(request.Kind),
			Operation: safeText(request.Operation),
			Target:    safeText(request.Target),
			Reason:    safeText(request.Reason),
		})
	}
	return &SemanticPolicyRoute{
		Visible:              true,
		Source:               safeText(defaultString(route.Source, "policy.capability")),
		Input:                safeText(route.Input),
		Candidate:            safeText(route.Candidate),
		Confidence:           route.Confidence,
		Reason:               safeText(route.Reason),
		NeededInput:          safeText(route.NeededInput),
		CurrentPhase:         safeText(route.CurrentPhase),
		RuntimeStatus:        safeText(route.RuntimeStatus),
		RecommendedSuccessor: safeText(route.RecommendedSuccessor),
		SuccessorValid:       route.SuccessorValid,
		SuccessorRejected:    route.SuccessorRejected,
		SuccessorReason:      safeText(route.SuccessorReason),
		TransitionClaimed:    route.TransitionClaimed,
		Executed:             route.Executed,
		SourceRefs:           refs,
		BoundaryRequests:     requests,
	}
}

func semanticVisionItems(vision *VisionView) []string {
	semantic := semanticVision(vision)
	if semantic == nil {
		return nil
	}
	items := []string{
		"source: " + semantic.Source,
		"capability: " + semantic.Capability,
		"signal: " + semantic.Signal,
		"phase: " + semantic.Phase,
		"artifact: " + semantic.ArtifactPath,
		"artifact_status: " + semantic.ArtifactStatus,
		"recommended_successor: " + semantic.RecommendedSuccessor,
		"successor_valid: " + boolLabel(semantic.SuccessorValid),
		"successor_rejected: " + boolLabel(semantic.SuccessorRejected),
		"transition_claimed: " + boolLabel(semantic.TransitionClaimed),
		"display_only: " + boolLabel(semantic.DisplayOnly),
	}
	if semantic.SuccessorReason != "" {
		items = append(items, "successor_reason: "+semantic.SuccessorReason)
	}
	if semantic.Summary != "" {
		items = append(items, "summary: "+semantic.Summary)
	}
	if semantic.NeededInput != "" {
		items = append(items, "needed_input: "+semantic.NeededInput)
	}
	if semantic.NorthStar != "" {
		items = append(items, "north_star: "+semantic.NorthStar)
	}
	for _, principle := range semantic.Principles {
		items = append(items, "principle: "+principle)
	}
	for _, goal := range semantic.LongTermGoals {
		items = append(items, "long_term_goal: "+goal)
	}
	for _, blocker := range semantic.Blockers {
		items = append(items, "blocker: "+blocker)
	}
	if semantic.NextAction != "" {
		items = append(items, "next_action: "+semantic.NextAction)
	}
	for _, ref := range semantic.ArtifactRefs {
		item := "artifact_ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			item += " path=" + ref.Path
		}
		items = append(items, item)
	}
	for _, request := range semantic.BoundaryRequests {
		item := "boundary_request: " + request.Kind
		if request.Operation != "" {
			item += " operation=" + request.Operation
		}
		if request.Target != "" {
			item += " target=" + request.Target
		}
		items = append(items, item)
	}
	for _, ref := range semantic.SourceRefs {
		item := "source_ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Excerpt != "" {
			item += " excerpt=" + ref.Excerpt
		}
		items = append(items, item)
	}
	return items
}

func semanticVision(vision *VisionView) *SemanticVision {
	if vision == nil {
		return nil
	}
	artifactRefs := make([]SemanticVisionArtifactRef, 0, len(vision.ArtifactRefs))
	for _, ref := range vision.ArtifactRefs {
		artifactRefs = append(artifactRefs, SemanticVisionArtifactRef{ID: safeText(ref.ID), Kind: safeText(ref.Kind), Path: safeText(ref.Path)})
	}
	refs := make([]SemanticVisionSourceRef, 0, len(vision.SourceRefs))
	for _, ref := range vision.SourceRefs {
		refs = append(refs, SemanticVisionSourceRef{ID: safeText(ref.ID), Kind: safeText(ref.Kind), Path: safeText(ref.Path), Command: safeText(ref.Command), Excerpt: safeText(ref.Excerpt)})
	}
	requests := make([]SemanticVisionBoundaryRequest, 0, len(vision.BoundaryRequests))
	for _, request := range vision.BoundaryRequests {
		requests = append(requests, SemanticVisionBoundaryRequest{Kind: safeText(request.Kind), Operation: safeText(request.Operation), Target: safeText(request.Target), Reason: safeText(request.Reason)})
	}
	return &SemanticVision{
		Visible:              true,
		Source:               safeText(defaultString(vision.Source, "app.vision")),
		Capability:           safeText(defaultString(vision.Capability, "vision")),
		Signal:               safeText(defaultString(vision.Signal, "complete")),
		Phase:                safeText(defaultString(vision.Phase, "envision")),
		Summary:              safeText(vision.Summary),
		NorthStar:            safeText(vision.NorthStar),
		Principles:           safeTextSlice(vision.Principles),
		LongTermGoals:        safeTextSlice(vision.LongTermGoals),
		Blockers:             safeTextSlice(vision.Blockers),
		NeededInput:          safeText(vision.NeededInput),
		NextAction:           safeText(vision.NextAction),
		ArtifactPath:         safeText(vision.ArtifactPath),
		ArtifactStatus:       safeText(vision.ArtifactStatus),
		RecommendedSuccessor: safeText(vision.RecommendedSuccessor),
		SuccessorValid:       vision.SuccessorValid,
		SuccessorRejected:    vision.SuccessorRejected,
		SuccessorReason:      safeText(vision.SuccessorReason),
		TransitionClaimed:    vision.TransitionClaimed,
		DisplayOnly:          vision.DisplayOnly,
		ArtifactRefs:         artifactRefs,
		SourceRefs:           refs,
		BoundaryRequests:     requests,
	}
}

func semanticAuditItems(audit *AuditView) []string {
	semantic := semanticAudit(audit)
	if semantic == nil {
		return nil
	}
	items := []string{
		"source: " + semantic.Source,
		"capability: " + semantic.Capability,
		"signal: " + semantic.Signal,
		"evidence: " + semantic.EvidenceState,
		"recommended_successor: " + semantic.RecommendedSuccessor,
		"successor_valid: " + boolLabel(semantic.SuccessorValid),
		"successor_rejected: " + boolLabel(semantic.SuccessorRejected),
		"transition_claimed: " + boolLabel(semantic.TransitionClaimed),
		"display_only: " + boolLabel(semantic.DisplayOnly),
	}
	if semantic.SuccessorReason != "" {
		items = append(items, "successor_reason: "+semantic.SuccessorReason)
	}
	if semantic.Summary != "" {
		items = append(items, "summary: "+semantic.Summary)
	}
	for _, finding := range semantic.Findings {
		items = append(items, "finding: "+finding.ID+" severity="+finding.Severity+" title="+finding.Title)
		if finding.Message != "" {
			items = append(items, "finding_message: "+finding.ID+" "+finding.Message)
		}
		if len(finding.SourceRefIDs) > 0 {
			items = append(items, "finding_source_refs: "+finding.ID+" "+strings.Join(finding.SourceRefIDs, ","))
		}
		for _, action := range finding.NextActions {
			items = append(items, "finding_next_action: "+finding.ID+" "+action)
		}
	}
	for _, action := range semantic.NextActions {
		items = append(items, "next_action: "+action)
	}
	for _, caveat := range semantic.Caveats {
		items = append(items, "caveat: "+caveat)
	}
	for _, ref := range semantic.SourceRefs {
		item := "source_ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			item += " path=" + ref.Path
		}
		if ref.Excerpt != "" {
			item += " excerpt=" + ref.Excerpt
		}
		items = append(items, item)
	}
	return items
}

func semanticAudit(audit *AuditView) *SemanticAudit {
	if audit == nil {
		return nil
	}
	findings := make([]SemanticAuditFinding, 0, len(audit.Findings))
	for _, finding := range audit.Findings {
		findings = append(findings, SemanticAuditFinding{
			ID:           safeText(finding.ID),
			Severity:     safeText(finding.Severity),
			Title:        safeText(finding.Title),
			Message:      safeText(finding.Message),
			SourceRefIDs: safeTextSlice(finding.SourceRefIDs),
			NextActions:  safeTextSlice(finding.NextActions),
		})
	}
	artifactRefs := make([]SemanticAuditArtifactRef, 0, len(audit.ArtifactRefs))
	for _, ref := range audit.ArtifactRefs {
		artifactRefs = append(artifactRefs, SemanticAuditArtifactRef{ID: safeText(ref.ID), Kind: safeText(ref.Kind), Path: safeText(ref.Path)})
	}
	refs := make([]SemanticAuditSourceRef, 0, len(audit.SourceRefs))
	for _, ref := range audit.SourceRefs {
		refs = append(refs, SemanticAuditSourceRef{ID: safeText(ref.ID), Kind: safeText(ref.Kind), Path: safeText(ref.Path), Command: safeText(ref.Command), Excerpt: safeText(ref.Excerpt)})
	}
	requests := make([]SemanticAuditBoundaryRequest, 0, len(audit.BoundaryRequests))
	for _, request := range audit.BoundaryRequests {
		requests = append(requests, SemanticAuditBoundaryRequest{Kind: safeText(request.Kind), Operation: safeText(request.Operation), Target: safeText(request.Target), Reason: safeText(request.Reason)})
	}
	return &SemanticAudit{
		Visible:              true,
		Source:               safeText(defaultString(audit.Source, "app.audit")),
		Capability:           safeText(defaultString(audit.Capability, "audit")),
		Signal:               safeText(defaultString(audit.Signal, "complete")),
		Summary:              safeText(audit.Summary),
		EvidenceState:        safeText(audit.EvidenceState),
		RecommendedSuccessor: safeText(audit.RecommendedSuccessor),
		SuccessorValid:       audit.SuccessorValid,
		SuccessorRejected:    audit.SuccessorRejected,
		SuccessorReason:      safeText(audit.SuccessorReason),
		TransitionClaimed:    audit.TransitionClaimed,
		DisplayOnly:          audit.DisplayOnly,
		Findings:             findings,
		NextActions:          safeTextSlice(audit.NextActions),
		Caveats:              safeTextSlice(audit.Caveats),
		ArtifactRefs:         artifactRefs,
		SourceRefs:           refs,
		BoundaryRequests:     requests,
	}
}

func semanticBuildItems(build *BuildView) []string {
	semantic := semanticBuild(build)
	if semantic == nil {
		return nil
	}
	items := []string{
		"source: " + semantic.Source,
		"capability: " + semantic.Capability,
		"signal: " + semantic.Signal,
		"plan_item: " + semantic.PlanItem.ID + " status=" + semantic.PlanItem.Status + " text=" + semantic.PlanItem.Text,
		"step: " + semantic.Step.ID + " status=" + semantic.Step.Status + " text=" + semantic.Step.Text,
		"tool: " + semantic.Operation.Name + " status=" + semantic.Operation.Status,
		"path: " + semantic.Operation.Path,
		"decision_source: " + semantic.Operation.DecisionSource,
		"decision_autonomy: " + semantic.Operation.DecisionAutonomy,
		"decision_allowed: " + boolLabel(semantic.Operation.DecisionAllowed),
		"approval_required: " + boolLabel(semantic.Operation.ApprovalRequired),
		"bytes_written: " + fmt.Sprint(semantic.Operation.BytesWritten),
		"recommended_successor: " + semantic.RecommendedSuccessor,
		"successor_valid: " + boolLabel(semantic.SuccessorValid),
		"transition_claimed: " + boolLabel(semantic.TransitionClaimed),
		"display_only: " + boolLabel(semantic.DisplayOnly),
	}
	if semantic.Summary != "" {
		items = append(items, "summary: "+semantic.Summary)
	}
	if semantic.FinalSummary != "" {
		items = append(items, "final_summary: "+semantic.FinalSummary)
	}
	for _, path := range semantic.ChangedPaths {
		items = append(items, "changed_path: "+path)
	}
	for _, blocker := range semantic.Blockers {
		items = append(items, "blocker: "+blocker)
	}
	for _, caveat := range semantic.Caveats {
		items = append(items, "caveat: "+caveat)
	}
	for _, ref := range semantic.ArtifactRefs {
		item := "artifact_ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			item += " path=" + ref.Path
		}
		items = append(items, item)
	}
	for _, request := range semantic.BoundaryRequests {
		item := "boundary_request: " + request.Kind
		if request.Operation != "" {
			item += " operation=" + request.Operation
		}
		if request.Target != "" {
			item += " target=" + request.Target
		}
		if request.Reason != "" {
			item += " reason=" + request.Reason
		}
		items = append(items, item)
	}
	for _, ref := range semantic.SourceRefs {
		item := "source_ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Excerpt != "" {
			item += " excerpt=" + ref.Excerpt
		}
		items = append(items, item)
	}
	return items
}

func semanticBuild(build *BuildView) *SemanticBuild {
	if build == nil {
		return nil
	}
	artifactRefs := make([]SemanticBuildArtifactRef, 0, len(build.ArtifactRefs))
	for _, ref := range build.ArtifactRefs {
		artifactRefs = append(artifactRefs, SemanticBuildArtifactRef{ID: safeText(ref.ID), Kind: safeText(ref.Kind), Path: safeText(ref.Path)})
	}
	refs := make([]SemanticBuildSourceRef, 0, len(build.SourceRefs))
	for _, ref := range build.SourceRefs {
		refs = append(refs, SemanticBuildSourceRef{ID: safeText(ref.ID), Kind: safeText(ref.Kind), Path: safeText(ref.Path), Command: safeText(ref.Command), Excerpt: safeText(ref.Excerpt)})
	}
	requests := make([]SemanticBuildBoundaryRequest, 0, len(build.BoundaryRequests))
	for _, request := range build.BoundaryRequests {
		requests = append(requests, SemanticBuildBoundaryRequest{Kind: safeText(request.Kind), Operation: safeText(request.Operation), Target: safeText(request.Target), Reason: safeText(request.Reason)})
	}
	return &SemanticBuild{
		Visible:              true,
		Source:               safeText(defaultString(build.Source, "app.build")),
		Capability:           safeText(defaultString(build.Capability, "build")),
		Signal:               safeText(defaultString(build.Signal, "complete")),
		Summary:              safeText(build.Summary),
		RecommendedSuccessor: safeText(build.RecommendedSuccessor),
		SuccessorValid:       build.SuccessorValid,
		TransitionClaimed:    build.TransitionClaimed,
		DisplayOnly:          build.DisplayOnly,
		PlanItem: SemanticBuildPlanItem{
			ID:     safeText(build.PlanItem.ID),
			Text:   safeText(build.PlanItem.Text),
			Status: safeText(build.PlanItem.Status),
		},
		Step: SemanticBuildStep{
			ID:     safeText(build.Step.ID),
			Text:   safeText(build.Step.Text),
			Status: safeText(build.Step.Status),
		},
		Operation: SemanticBuildOperation{
			Name:             safeText(build.Operation.Name),
			Status:           safeText(build.Operation.Status),
			Path:             safeText(build.Operation.Path),
			ExpectedEffect:   safeText(build.Operation.ExpectedEffect),
			DecisionSource:   safeText(build.Operation.DecisionSource),
			DecisionAutonomy: safeText(build.Operation.DecisionAutonomy),
			DecisionAllowed:  build.Operation.DecisionAllowed,
			ApprovalRequired: build.Operation.ApprovalRequired,
			BytesWritten:     build.Operation.BytesWritten,
			ErrorKind:        safeText(build.Operation.ErrorKind),
			ErrorMessage:     safeText(build.Operation.ErrorMessage),
		},
		ChangedPaths:     safeTextSlice(build.ChangedPaths),
		Blockers:         safeTextSlice(build.Blockers),
		Caveats:          safeTextSlice(build.Caveats),
		FinalSummary:     safeText(build.FinalSummary),
		ArtifactRefs:     artifactRefs,
		SourceRefs:       refs,
		BoundaryRequests: requests,
	}
}

func semanticPlanItems(plan *PlanView) []string {
	semantic := semanticPlan(plan)
	if semantic == nil {
		return nil
	}
	items := []string{
		"source: " + semantic.Source,
		"capability: " + semantic.Capability,
		"signal: " + semantic.Signal,
		"title: " + semantic.Title,
		"scope: " + semantic.Scope,
		"artifact_path: " + semantic.ArtifactPath,
		"artifact_status: " + semantic.ArtifactStatus,
		"recommended_successor: " + semantic.RecommendedSuccessor,
		"successor_valid: " + boolLabel(semantic.SuccessorValid),
		"transition_claimed: " + boolLabel(semantic.TransitionClaimed),
		"display_only: " + boolLabel(semantic.DisplayOnly),
	}
	if semantic.Summary != "" {
		items = append(items, "summary: "+semantic.Summary)
	}
	for _, item := range semantic.Items {
		items = append(items, "item: "+item.ID+" status="+item.Status+" done="+boolLabel(item.Done)+" text="+item.Text)
		for _, acceptance := range item.Acceptance {
			items = append(items, "acceptance: "+item.ID+" "+acceptance)
		}
		if len(item.SourceRefIDs) > 0 {
			items = append(items, "item_source_refs: "+item.ID+" "+strings.Join(item.SourceRefIDs, ","))
		}
	}
	for _, blocker := range semantic.Blockers {
		items = append(items, "blocker: "+blocker)
	}
	if semantic.NextAction != "" {
		items = append(items, "next_action: "+semantic.NextAction)
	}
	for _, ref := range semantic.ArtifactRefs {
		item := "artifact_ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			item += " path=" + ref.Path
		}
		items = append(items, item)
	}
	for _, request := range semantic.BoundaryRequests {
		item := "boundary_request: " + request.Kind
		if request.Operation != "" {
			item += " operation=" + request.Operation
		}
		if request.Target != "" {
			item += " target=" + request.Target
		}
		if request.Reason != "" {
			item += " reason=" + request.Reason
		}
		items = append(items, item)
	}
	for _, ref := range semantic.SourceRefs {
		item := "source_ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			item += " path=" + ref.Path
		}
		if ref.Command != "" {
			item += " command=" + ref.Command
		}
		if ref.Excerpt != "" {
			item += " excerpt=" + ref.Excerpt
		}
		items = append(items, item)
	}
	return items
}

func semanticPlan(plan *PlanView) *SemanticPlan {
	if plan == nil {
		return nil
	}
	items := make([]SemanticPlanItem, 0, len(plan.Items))
	for _, item := range plan.Items {
		items = append(items, SemanticPlanItem{ID: safeText(item.ID), Text: safeText(item.Text), Status: safeText(item.Status), Done: item.Done, Acceptance: safeTextSlice(item.Acceptance), SourceRefIDs: safeTextSlice(item.SourceRefIDs)})
	}
	artifactRefs := make([]SemanticPlanArtifactRef, 0, len(plan.ArtifactRefs))
	for _, ref := range plan.ArtifactRefs {
		artifactRefs = append(artifactRefs, SemanticPlanArtifactRef{ID: safeText(ref.ID), Kind: safeText(ref.Kind), Path: safeText(ref.Path)})
	}
	refs := make([]SemanticPlanSourceRef, 0, len(plan.SourceRefs))
	for _, ref := range plan.SourceRefs {
		refs = append(refs, SemanticPlanSourceRef{ID: safeText(ref.ID), Kind: safeText(ref.Kind), Path: safeText(ref.Path), Command: safeText(ref.Command), Excerpt: safeText(ref.Excerpt)})
	}
	requests := make([]SemanticPlanBoundaryRequest, 0, len(plan.BoundaryRequests))
	for _, request := range plan.BoundaryRequests {
		requests = append(requests, SemanticPlanBoundaryRequest{Kind: safeText(request.Kind), Operation: safeText(request.Operation), Target: safeText(request.Target), Reason: safeText(request.Reason)})
	}
	return &SemanticPlan{
		Visible:              true,
		Source:               safeText(defaultString(plan.Source, "app.plan")),
		Capability:           safeText(defaultString(plan.Capability, "plan")),
		Signal:               safeText(defaultString(plan.Signal, "complete")),
		Title:                safeText(plan.Title),
		Scope:                safeText(plan.Scope),
		Summary:              safeText(plan.Summary),
		ArtifactPath:         safeText(plan.ArtifactPath),
		ArtifactStatus:       safeText(defaultString(plan.ArtifactStatus, "available")),
		RecommendedSuccessor: safeText(plan.RecommendedSuccessor),
		SuccessorValid:       plan.SuccessorValid,
		TransitionClaimed:    plan.TransitionClaimed,
		DisplayOnly:          plan.DisplayOnly,
		Items:                items,
		Blockers:             safeTextSlice(plan.Blockers),
		NextAction:           safeText(plan.NextAction),
		ArtifactRefs:         artifactRefs,
		SourceRefs:           refs,
		BoundaryRequests:     requests,
	}
}

func semanticBriefItems(brief *BriefView) []string {
	semantic := semanticBrief(brief)
	if semantic == nil {
		return nil
	}
	items := []string{
		"source: " + semantic.Source,
		"capability: " + semantic.Capability,
		"signal: " + semantic.Signal,
		"current_phase: " + semantic.CurrentPhase,
		"runtime_status: " + semantic.RuntimeStatus,
		"transition_claimed: " + boolLabel(semantic.TransitionClaimed),
		"display_only: " + boolLabel(semantic.DisplayOnly),
	}
	if semantic.Summary != "" {
		items = append(items, "summary: "+semantic.Summary)
	}
	for _, gap := range semantic.KnownGaps {
		items = append(items, "known_gap: "+gap)
	}
	if semantic.SuggestedNextAction != "" {
		items = append(items, "suggested_next_action: "+semantic.SuggestedNextAction)
	}
	for _, request := range semantic.BoundaryRequests {
		item := "boundary_request: " + request.Kind
		if request.Operation != "" {
			item += " operation=" + request.Operation
		}
		if request.Target != "" {
			item += " target=" + request.Target
		}
		if request.Reason != "" {
			item += " reason=" + request.Reason
		}
		items = append(items, item)
	}
	for _, ref := range semantic.SourceRefs {
		item := "source_ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			item += " path=" + ref.Path
		}
		if ref.Command != "" {
			item += " command=" + ref.Command
		}
		if ref.Excerpt != "" {
			item += " excerpt=" + ref.Excerpt
		}
		items = append(items, item)
	}
	return items
}

func semanticBrief(brief *BriefView) *SemanticBrief {
	if brief == nil {
		return nil
	}
	refs := make([]SemanticBriefSourceRef, 0, len(brief.SourceRefs))
	for _, ref := range brief.SourceRefs {
		refs = append(refs, SemanticBriefSourceRef{ID: safeText(ref.ID), Kind: safeText(ref.Kind), Path: safeText(ref.Path), Command: safeText(ref.Command), Excerpt: safeText(ref.Excerpt)})
	}
	requests := make([]SemanticBriefBoundaryRequest, 0, len(brief.BoundaryRequests))
	for _, request := range brief.BoundaryRequests {
		requests = append(requests, SemanticBriefBoundaryRequest{Kind: safeText(request.Kind), Operation: safeText(request.Operation), Target: safeText(request.Target), Reason: safeText(request.Reason)})
	}
	return &SemanticBrief{
		Visible:             true,
		Source:              safeText(defaultString(brief.Source, "app.brief")),
		Capability:          safeText(defaultString(brief.Capability, "brief")),
		Signal:              safeText(defaultString(brief.Signal, "complete")),
		Summary:             safeText(brief.Summary),
		CurrentPhase:        safeText(brief.CurrentPhase),
		RuntimeStatus:       safeText(brief.RuntimeStatus),
		KnownGaps:           safeTextSlice(brief.KnownGaps),
		SuggestedNextAction: safeText(brief.SuggestedNextAction),
		TransitionClaimed:   brief.TransitionClaimed,
		DisplayOnly:         brief.DisplayOnly,
		SourceRefs:          refs,
		BoundaryRequests:    requests,
	}
}

func semanticUtilityItems(utility *UtilityView) []string {
	semantic := semanticUtility(utility)
	if semantic == nil {
		return nil
	}
	items := []string{
		"status: " + semantic.Status,
		"job: " + semantic.JobKind + " " + semantic.JobID,
		"model: " + semantic.Model,
		"read_only: " + boolLabel(semantic.ReadOnly),
		"file_mutation: " + boolLabel(semantic.Safety.FileMutation),
		"git_mutation: " + boolLabel(semantic.Safety.GitMutation),
		"project_artifact_mutation: " + boolLabel(semantic.Safety.ProjectArtifactMutation),
		"permission_approval: " + boolLabel(semantic.Safety.ApprovalGrant),
		"workflow_phase_transition: " + boolLabel(semantic.Safety.WorkflowPhaseTransition),
		"final_judgment: " + boolLabel(semantic.Safety.FinalJudgment),
		"context_refresh: " + boolLabel(semantic.Safety.ContextRefresh),
		"context_compaction: " + boolLabel(semantic.Safety.ContextCompaction),
		"context_rewrite: " + boolLabel(semantic.Safety.ContextRewrite),
		"display-only",
	}
	if semantic.Summary != "" {
		items = append(items, "summary: "+semantic.Summary)
	}
	if semantic.PreparedContext != nil {
		items = append(items, "prepared_context: "+semantic.PreparedContext.Summary+" refs="+strings.Join(semantic.PreparedContext.EvidenceRefIDs, ","))
		items = append(items, "prepared_context_non_authoritative: "+boolLabel(semantic.PreparedContext.NonAuthoritative))
		for _, caveat := range semantic.PreparedContext.Caveats {
			items = append(items, "prepared_context_caveat: "+caveat)
		}
	}
	if semantic.StaleContext != nil {
		items = append(items, "stale_context_status: "+semantic.StaleContext.Status)
		items = append(items, "stale_context_summary: "+semantic.StaleContext.Summary+" refs="+strings.Join(semantic.StaleContext.EvidenceRefIDs, ","))
		for _, caveat := range semantic.StaleContext.Caveats {
			items = append(items, "stale_context_caveat: "+caveat)
		}
		if semantic.StaleContext.SuggestedNextAction != "" {
			items = append(items, "suggested_next_action: "+semantic.StaleContext.SuggestedNextAction)
		}
	}
	if semantic.SummaryRefresh != nil {
		items = append(items, "summary_refresh_status: "+semantic.SummaryRefresh.Status)
		items = append(items, "summary_refresh_original: "+semantic.SummaryRefresh.OriginalSummary)
		items = append(items, "summary_refresh_refreshed: "+semantic.SummaryRefresh.RefreshedSummary+" refs="+strings.Join(semantic.SummaryRefresh.SourceRefIDs, ","))
		items = append(items, "summary_refresh_confidence: "+semantic.SummaryRefresh.Confidence)
		for _, detail := range semantic.SummaryRefresh.ExactDetails {
			items = append(items, "summary_refresh_detail: "+detail)
		}
		for _, caveat := range semantic.SummaryRefresh.Caveats {
			items = append(items, "summary_refresh_caveat: "+caveat)
		}
	}
	for _, suggestion := range semantic.Suggestions {
		items = append(items, "suggestion: "+suggestion.Text+" refs="+strings.Join(suggestion.EvidenceRefIDs, ","))
	}
	for _, ref := range semantic.EvidenceRefs {
		items = append(items, "evidence: "+ref.ID+" "+ref.Kind+" "+ref.Source+" "+ref.Detail)
	}
	for _, caveat := range semantic.Caveats {
		items = append(items, "caveat: "+caveat)
	}
	if semantic.DeniedReason != "" {
		items = append(items, "denied: "+semantic.DeniedReason+" "+semantic.DeniedDetail)
	}
	return items
}

func semanticUtility(utility *UtilityView) *SemanticUtility {
	if utility == nil {
		return nil
	}
	var prepared *SemanticUtilityPreparedContext
	if utility.PreparedContext.Summary != "" || len(utility.PreparedContext.EvidenceRefIDs) > 0 || len(utility.PreparedContext.Caveats) > 0 || utility.PreparedContext.NonAuthoritative {
		prepared = &SemanticUtilityPreparedContext{
			Summary:          safeText(utility.PreparedContext.Summary),
			EvidenceRefIDs:   safeTextSlice(utility.PreparedContext.EvidenceRefIDs),
			Caveats:          safeTextSlice(utility.PreparedContext.Caveats),
			NonAuthoritative: utility.PreparedContext.NonAuthoritative,
		}
	}
	var stale *SemanticUtilityStaleContext
	if utility.StaleContext.Status != "" || utility.StaleContext.Summary != "" || len(utility.StaleContext.EvidenceRefIDs) > 0 || len(utility.StaleContext.Caveats) > 0 || utility.StaleContext.SuggestedNextAction != "" {
		stale = &SemanticUtilityStaleContext{
			Status:              safeText(utility.StaleContext.Status),
			Summary:             safeText(utility.StaleContext.Summary),
			EvidenceRefIDs:      safeTextSlice(utility.StaleContext.EvidenceRefIDs),
			Caveats:             safeTextSlice(utility.StaleContext.Caveats),
			SuggestedNextAction: safeText(utility.StaleContext.SuggestedNextAction),
		}
	}
	var summaryRefresh *SemanticUtilitySummaryRefresh
	if utility.SummaryRefresh.Status != "" || utility.SummaryRefresh.RefreshedSummary != "" || len(utility.SummaryRefresh.SourceRefIDs) > 0 || len(utility.SummaryRefresh.ExactDetails) > 0 || len(utility.SummaryRefresh.Caveats) > 0 {
		summaryRefresh = &SemanticUtilitySummaryRefresh{
			Status:           safeText(utility.SummaryRefresh.Status),
			OriginalSummary:  safeText(utility.SummaryRefresh.OriginalSummary),
			RefreshedSummary: safeText(utility.SummaryRefresh.RefreshedSummary),
			SourceRefIDs:     safeTextSlice(utility.SummaryRefresh.SourceRefIDs),
			ExactDetails:     safeTextSlice(utility.SummaryRefresh.ExactDetails),
			Confidence:       safeText(utility.SummaryRefresh.Confidence),
			Caveats:          safeTextSlice(utility.SummaryRefresh.Caveats),
		}
	}
	suggestions := make([]SemanticUtilitySuggestion, 0, len(utility.Suggestions))
	for _, suggestion := range utility.Suggestions {
		suggestions = append(suggestions, SemanticUtilitySuggestion{Text: safeText(suggestion.Text), EvidenceRefIDs: safeTextSlice(suggestion.EvidenceRefIDs)})
	}
	evidence := make([]SemanticUtilityEvidence, 0, len(utility.EvidenceRefs))
	for _, ref := range utility.EvidenceRefs {
		evidence = append(evidence, SemanticUtilityEvidence{ID: safeText(ref.ID), Kind: safeText(ref.Kind), Source: safeText(ref.Source), Detail: safeText(ref.Detail)})
	}
	return &SemanticUtility{
		Source:          safeText(defaultString(utility.Source, "app.utility")),
		Status:          safeText(defaultString(utility.Status, "idle")),
		JobID:           safeText(utility.JobID),
		JobKind:         safeText(utility.JobKind),
		Model:           safeText(utility.Model),
		Summary:         safeText(utility.Summary),
		PreparedContext: prepared,
		StaleContext:    stale,
		SummaryRefresh:  summaryRefresh,
		Suggestions:     suggestions,
		EvidenceRefs:    evidence,
		Caveats:         safeTextSlice(utility.Caveats),
		DeniedReason:    safeText(utility.DeniedReason),
		DeniedDetail:    safeText(utility.DeniedDetail),
		ReadOnly:        utility.ReadOnly,
		Safety: SemanticUtilitySafety{
			FileMutation:            utility.Safety.FileMutation,
			GitMutation:             utility.Safety.GitMutation,
			ProjectArtifactMutation: utility.Safety.ProjectArtifactMutation,
			ApprovalGrant:           utility.Safety.ApprovalGrant,
			WorkflowPhaseTransition: utility.Safety.WorkflowPhaseTransition,
			FinalJudgment:           utility.Safety.FinalJudgment,
			ContextRefresh:          utility.Safety.ContextRefresh,
			ContextCompaction:       utility.Safety.ContextCompaction,
			ContextRewrite:          utility.Safety.ContextRewrite,
		},
	}
}

func semanticCompactItems(compact *CompactView) []string {
	semantic := semanticCompact(compact)
	if semantic == nil {
		return nil
	}
	items := []string{
		"source: " + semantic.Source,
		"mode: " + semantic.Mode,
		"status: " + semantic.Status,
	}
	if semantic.Summary != "" {
		items = append(items, "summary: "+semantic.Summary)
	}
	if semantic.OriginalMeter != "" {
		items = append(items, "original_meter: "+semantic.OriginalMeter)
	}
	if semantic.Meter != "" {
		items = append(items, "meter: "+semantic.Meter)
	}
	for _, caveat := range semantic.Caveats {
		items = append(items, "caveat: "+caveat)
	}
	for _, ref := range semantic.SourceRefs {
		item := "source_ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			item += " path=" + ref.Path
		}
		if ref.Command != "" {
			item += " command=" + ref.Command
		}
		if ref.Excerpt != "" {
			item += " excerpt=" + ref.Excerpt
		}
		items = append(items, item)
	}
	items = append(items, "app-owned", "display-only")
	return items
}

func semanticCompact(compact *CompactView) *SemanticCompact {
	if compact == nil {
		return nil
	}
	refs := make([]SemanticContextSourceRef, 0, len(compact.SourceRefs))
	for _, ref := range compact.SourceRefs {
		refs = append(refs, SemanticContextSourceRef{
			ID:        safeText(ref.ID),
			Kind:      safeText(ref.Kind),
			Label:     safeText(ref.Label),
			Path:      safeText(ref.Path),
			LineStart: ref.LineStart,
			LineEnd:   ref.LineEnd,
			Command:   safeText(ref.Command),
			Stream:    safeText(ref.Stream),
			Excerpt:   safeText(ref.Excerpt),
		})
	}
	return &SemanticCompact{
		Source:        safeText(defaultString(compact.Source, "app.compact")),
		Mode:          safeText(defaultString(compact.Mode, "manual")),
		Status:        safeText(defaultString(compact.Status, "completed")),
		Summary:       safeText(compact.Summary),
		Meter:         safeText(compact.Meter),
		OriginalMeter: safeText(compact.OriginalMeter),
		Caveats:       safeTextSlice(compact.Caveats),
		SourceRefs:    refs,
	}
}

func semanticContextItems(contextView *ContextView) []string {
	semantic := semanticContext(contextView)
	if semantic == nil {
		return nil
	}
	items := []string{
		"source: " + semantic.Source,
		"status: " + semantic.Status,
		"meter: " + semantic.Meter,
	}
	for _, block := range semantic.Blocks {
		items = append(items, "block: "+block.ID+" "+block.Kind+" "+block.Title)
		if block.Text != "" {
			items = append(items, "block_text: "+block.Text)
		}
		for _, refID := range block.SourceRefIDs {
			items = append(items, "block_ref: "+block.ID+" "+refID)
		}
	}
	for _, claim := range semantic.Claims {
		items = append(items, "claim: "+claim.Text)
		for _, refID := range claim.SourceRefIDs {
			items = append(items, "claim_ref: "+claim.Text+" -> "+refID)
		}
	}
	for _, ref := range semantic.SourceRefs {
		item := "source_ref: " + ref.ID + " kind=" + ref.Kind
		if ref.Path != "" {
			item += " path=" + ref.Path
		}
		if ref.Command != "" {
			item += " command=" + ref.Command
		}
		if ref.Stream != "" {
			item += " stream=" + ref.Stream
		}
		if ref.Excerpt != "" {
			item += " excerpt=" + ref.Excerpt
		}
		items = append(items, item)
	}
	for _, warning := range semantic.Warnings {
		items = append(items, "warning: "+warning)
	}
	items = append(items, "app-owned", "display-only")
	return items
}

func semanticContext(contextView *ContextView) *SemanticContext {
	if contextView == nil {
		return nil
	}
	blocks := make([]SemanticContextBlock, 0, len(contextView.Blocks))
	for _, block := range contextView.Blocks {
		blocks = append(blocks, SemanticContextBlock{
			ID:           safeText(block.ID),
			Kind:         safeText(block.Kind),
			Title:        safeText(block.Title),
			Text:         safeText(block.Text),
			SourceRefIDs: safeTextSlice(block.SourceRefIDs),
		})
	}
	claims := make([]SemanticContextClaim, 0, len(contextView.Claims))
	for _, claim := range contextView.Claims {
		claims = append(claims, SemanticContextClaim{Text: safeText(claim.Text), SourceRefIDs: safeTextSlice(claim.SourceRefIDs)})
	}
	refs := make([]SemanticContextSourceRef, 0, len(contextView.SourceRefs))
	for _, ref := range contextView.SourceRefs {
		refs = append(refs, SemanticContextSourceRef{
			ID:        safeText(ref.ID),
			Kind:      safeText(ref.Kind),
			Label:     safeText(ref.Label),
			Path:      safeText(ref.Path),
			LineStart: ref.LineStart,
			LineEnd:   ref.LineEnd,
			Command:   safeText(ref.Command),
			Stream:    safeText(ref.Stream),
			Excerpt:   safeText(ref.Excerpt),
		})
	}
	return &SemanticContext{
		Source:     safeText(defaultString(contextView.Source, "app.context")),
		Status:     safeText(defaultString(contextView.Status, "ready")),
		Meter:      safeText(contextView.Meter),
		Blocks:     blocks,
		Claims:     claims,
		SourceRefs: refs,
		Warnings:   safeTextSlice(contextView.Warnings),
	}
}

func semanticRecoveryItems(recovery *RecoveryView) []string {
	semantic := semanticRecovery(recovery)
	if semantic == nil {
		return nil
	}
	items := []string{
		"command: " + semantic.Command,
		"status: " + semantic.Status,
		"action: " + semantic.Action,
		"completed: " + boolLabel(semantic.Completed),
		"redo_available: " + boolLabel(semantic.RedoAvailable),
	}
	if semantic.TargetEventID != "" {
		items = append(items, "target_event_id: "+semantic.TargetEventID)
	}
	if len(semantic.Paths) > 0 {
		items = append(items, "paths: "+strings.Join(semantic.Paths, ","))
	}
	if semantic.PreviousVersion != "" {
		items = append(items, "previous_version: "+semantic.PreviousVersion)
	}
	if semantic.NewVersion != "" {
		items = append(items, "new_version: "+semantic.NewVersion)
	}
	if semantic.RedoAction != "" {
		items = append(items, "redo_action: "+semantic.RedoAction)
	}
	if semantic.Reason != "" {
		items = append(items, "reason: "+semantic.Reason)
	}
	if semantic.ErrorKind != "" {
		items = append(items, "error_kind: "+semantic.ErrorKind)
	}
	if semantic.ErrorMessage != "" {
		items = append(items, "error_message: "+semantic.ErrorMessage)
	}
	items = appendDecisionItems(items, semantic.Decision)
	items = append(items, "app-owned", "display-only")
	return items
}

func semanticRecovery(recovery *RecoveryView) *SemanticRecovery {
	if recovery == nil {
		return nil
	}
	status := safeText(recovery.Status)
	if status == "" {
		status = "unsupported"
	}
	return &SemanticRecovery{
		Command:         safeText(recovery.Command),
		Status:          status,
		TargetEventID:   safeText(recovery.TargetEventID),
		Action:          safeText(recovery.Action),
		Paths:           safeTextSlice(recovery.Paths),
		PreviousVersion: safeText(recovery.PreviousVersion),
		NewVersion:      safeText(recovery.NewVersion),
		RedoAvailable:   recovery.RedoAvailable,
		RedoAction:      safeText(recovery.RedoAction),
		Reason:          safeText(recovery.Reason),
		ErrorKind:       safeText(recovery.ErrorKind),
		ErrorMessage:    safeText(recovery.ErrorMessage),
		Decision:        semanticDecision(recovery.Decision),
		Completed:       status == "completed" || status == "failed" || status == "unsupported",
	}
}

func semanticMutationItems(mutation *MutationView) []string {
	semantic := semanticMutation(mutation)
	if semantic == nil {
		return nil
	}
	items := []string{
		"tool_name: " + semantic.Name,
		"status: " + semantic.Status,
		"path: " + semantic.Path,
		"completed: " + boolLabel(semantic.Completed),
		"previous_exists: " + boolLabel(semantic.PreviousExists),
		fmt.Sprintf("bytes_written: %d", semantic.BytesWritten),
	}
	if semantic.ExpectedEffect != "" {
		items = append(items, "expected_effect: "+semantic.ExpectedEffect)
	}
	if semantic.PreviousVersion != "" {
		items = append(items, "previous_version: "+semantic.PreviousVersion)
	}
	if semantic.NewVersion != "" {
		items = append(items, "new_version: "+semantic.NewVersion)
	}
	if semantic.ReplacementCount > 0 {
		items = append(items, fmt.Sprintf("replacement_count: %d", semantic.ReplacementCount))
	}
	if semantic.ErrorKind != "" {
		items = append(items, "error_kind: "+semantic.ErrorKind)
	}
	if semantic.ErrorMessage != "" {
		items = append(items, "error_message: "+semantic.ErrorMessage)
	}
	items = appendDecisionItems(items, semantic.Decision)
	items = append(items, "app-owned", "display-only")
	return items
}

func semanticMutation(mutation *MutationView) *SemanticMutation {
	if mutation == nil {
		return nil
	}
	status := safeText(mutation.Status)
	if status == "" {
		status = "completed"
	}
	completed := status == "completed" || status == "failed" || status == "denied"
	return &SemanticMutation{
		Name:                  safeText(mutation.Name),
		Status:                status,
		Path:                  safeDecisionTarget(mutation.Path),
		ExpectedEffect:        safeText(mutation.ExpectedEffect),
		PreviousVersion:       safeText(mutation.PreviousVersion),
		NewVersion:            safeText(mutation.NewVersion),
		PreviousExists:        mutation.PreviousExists,
		BytesWritten:          mutation.BytesWritten,
		ReplacementCount:      mutation.ReplacementCount,
		ResolvedPathAvailable: mutation.ResolvedPathAvailable,
		ErrorKind:             safeText(mutation.ErrorKind),
		ErrorMessage:          safeText(mutation.ErrorMessage),
		Decision:              semanticDecision(mutation.Decision),
		Completed:             completed,
	}
}

func semanticFetchItems(fetch *FetchView) []string {
	semantic := semanticFetch(fetch)
	if semantic == nil {
		return nil
	}
	items := []string{
		"tool_name: " + semantic.Name,
		"status: " + semantic.Status,
		"read_only: " + boolLabel(semantic.ReadOnly),
		"url: " + semantic.URL,
		"method: " + semantic.Method,
		"completed: " + boolLabel(semantic.Completed),
	}
	if semantic.ExpectedEffect != "" {
		items = append(items, "expected_effect: "+semantic.ExpectedEffect)
	}
	if semantic.Completed && semantic.HTTPStatusCode > 0 {
		items = append(items, fmt.Sprintf("http_status_code: %d", semantic.HTTPStatusCode))
	}
	if semantic.HTTPStatus != "" {
		items = append(items, "http_status: "+semantic.HTTPStatus)
	}
	if semantic.ContentType != "" {
		items = append(items, "content_type: "+semantic.ContentType)
	}
	for _, line := range semantic.PreviewLines {
		items = append(items, "preview_line: "+line)
	}
	if semantic.Completed {
		items = append(items,
			"preview_truncated: "+boolLabel(semantic.PreviewTruncated),
			"omitted_bytes_known: "+boolLabel(semantic.OmittedBytesKnown),
		)
		if semantic.OmittedBytesKnown {
			items = append(items, fmt.Sprintf("omitted_bytes: %d", semantic.OmittedBytes))
		}
	}
	if semantic.TruncationMarker != "" {
		items = append(items, "truncation_marker: "+semantic.TruncationMarker)
	}
	if semantic.ErrorKind != "" {
		items = append(items, "error_kind: "+semantic.ErrorKind)
	}
	if semantic.ErrorMessage != "" {
		items = append(items, "error_message: "+semantic.ErrorMessage)
	}
	items = appendDecisionItems(items, semantic.Decision)
	items = append(items, "app-owned", "display-only")
	return items
}

func semanticFetch(fetch *FetchView) *SemanticFetch {
	if fetch == nil {
		return nil
	}
	status := safeText(fetch.Status)
	if status == "" {
		status = "running"
	}
	completed := status != "running"
	if fetch.ErrorKind != "" {
		completed = true
	}
	name := safeText(fetch.Name)
	if name == "" {
		name = "fetch"
	}
	method := safeText(fetch.Method)
	if method == "" {
		method = "GET"
	}
	semantic := &SemanticFetch{
		Name:              name,
		Status:            status,
		ReadOnly:          fetch.ReadOnly,
		URL:               safeFetchURL(fetch.URL),
		Method:            method,
		ExpectedEffect:    safeText(fetch.ExpectedEffect),
		HTTPStatusCode:    fetch.HTTPStatusCode,
		HTTPStatus:        safeText(fetch.HTTPStatus),
		ContentType:       safeText(fetch.ContentType),
		PreviewLines:      safeFetchPreviewLines(fetch.PreviewLines),
		PreviewTruncated:  fetch.PreviewTruncated,
		OmittedBytesKnown: fetch.OmittedBytesKnown,
		OmittedBytes:      fetch.OmittedBytes,
		TruncationMarker:  safeText(fetch.TruncationMarker),
		DurationMillis:    fetch.DurationMillis,
		ErrorKind:         safeText(fetch.ErrorKind),
		ErrorMessage:      safeText(fetch.ErrorMessage),
		Decision:          semanticDecision(fetch.Decision),
		Completed:         completed,
	}
	if !semantic.Completed {
		semantic.ExpectedEffect = ""
		semantic.HTTPStatusCode = 0
		semantic.HTTPStatus = ""
		semantic.ContentType = ""
		semantic.PreviewLines = nil
		semantic.PreviewTruncated = false
		semantic.OmittedBytesKnown = false
		semantic.OmittedBytes = 0
		semantic.TruncationMarker = ""
		semantic.DurationMillis = 0
		semantic.ErrorKind = ""
		semantic.ErrorMessage = ""
		semantic.Decision = nil
	}
	return semantic
}

func appendDecisionItems(items []string, decision *SemanticDecision) []string {
	if decision == nil {
		return items
	}
	items = append(items,
		"decision_source: "+decision.Source,
		"decision: "+decisionLabel(decision.Allowed),
		"decision_automatic: "+boolLabel(decision.Automatic),
		"approval_required: "+boolLabel(decision.ApprovalRequired),
		"decision_autonomy: "+decision.Autonomy,
		"operation_kind: "+decision.OperationKind,
	)
	if decision.Name != "" {
		items = append(items, "decision_tool: "+decision.Name)
	}
	if decision.Target != "" {
		items = append(items, "decision_target: "+decision.Target)
	}
	if len(decision.Command) > 0 {
		items = append(items, "decision_command: "+strings.Join(decision.Command, " "))
	}
	if decision.WorkingDir != "" {
		items = append(items, "decision_working_dir: "+decision.WorkingDir)
	}
	if decision.ExpectedEffect != "" {
		items = append(items, "decision_expected_effect: "+decision.ExpectedEffect)
	}
	items = append(items, "decision_reversible: "+boolLabel(decision.Reversible))
	if decision.RunID != "" {
		items = append(items, "decision_run_id: "+decision.RunID)
	}
	if decision.Capability != "" {
		items = append(items, "decision_capability: "+decision.Capability)
	}
	if decision.Reason != "" {
		items = append(items, "decision_reason: "+decision.Reason)
	}
	return items
}

func semanticDecision(decision *DecisionView) *SemanticDecision {
	if decision == nil || decision.Source == "" {
		return nil
	}
	workingDir := ""
	if decision.WorkingDir != "" {
		workingDir = safeCommandPath(decision.WorkingDir)
	}
	return &SemanticDecision{
		Autonomy:         safeText(decision.Autonomy),
		Source:           safeText(decision.Source),
		Allowed:          decision.Allowed,
		Automatic:        decision.Automatic,
		ApprovalRequired: decision.ApprovalRequired,
		Reason:           safeText(decision.Reason),
		OperationKind:    safeText(decision.OperationKind),
		Name:             safeText(decision.Name),
		Target:           safeDecisionTarget(decision.Target),
		Command:          safeCommandArgv(decision.Command),
		WorkingDir:       workingDir,
		ExpectedEffect:   safeText(decision.ExpectedEffect),
		Reversible:       decision.Reversible,
		RunID:            safeText(decision.RunID),
		Capability:       safeText(decision.Capability),
	}
}

func safeDecisionTarget(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return safeFetchURL(value)
	}
	return safeReadTargetPath(value)
}

func safeCommandOutputLines(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	const maxPreviewLines = 12
	limit := len(lines)
	if limit > maxPreviewLines {
		limit = maxPreviewLines
	}
	items := make([]string, 0, limit)
	for _, line := range lines[:limit] {
		items = append(items, safeCommandOutputLine(line))
	}
	return items
}

func safeCommandOutputLine(value string) string {
	value = stripTerminalControls(value)
	value = secretLikeText.ReplaceAllString(value, "[redacted]")
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return ""
	}
	fields := strings.Fields(value)
	for i, field := range fields {
		fields[i] = safeCommandOutputField(field)
	}
	return limitTextBytes(strings.Join(fields, " "), maxDisplayTextBytes)
}

func safeCommandOutputField(field string) string {
	slashPath := strings.ReplaceAll(field, "\\", "/")
	if strings.HasPrefix(slashPath, "/") || strings.HasPrefix(slashPath, "~") || strings.HasPrefix(slashPath, "$HOME") || strings.HasPrefix(slashPath, "${HOME}") || strings.HasPrefix(slashPath, "$XDG_") || strings.HasPrefix(slashPath, "${XDG_") || (strings.Contains(slashPath, "../") || strings.Contains(slashPath, "/..")) || strings.Contains(slashPath, "/\x2eaila") || strings.Contains(slashPath, "/\x2eagentera") || strings.Contains(slashPath, "/\x2econfig") || strings.HasPrefix(slashPath, "\x2eaila") || strings.HasPrefix(slashPath, "\x2eagentera") || strings.HasPrefix(slashPath, "\x2econfig") {
		return "[path-redacted]"
	}
	return field
}

func safeCommandArgv(argv []string) []string {
	if len(argv) == 0 {
		return nil
	}
	items := make([]string, 0, len(argv))
	for _, arg := range argv {
		items = append(items, safeText(arg))
	}
	return items
}

func safeCommandPath(value string) string {
	if value == "" {
		return "."
	}
	return safeReadTargetPath(value)
}

func safeFetchURL(value string) string {
	value = stripTerminalControls(strings.TrimSpace(value))
	value = secretLikeText.ReplaceAllString(value, "[redacted]")
	if value == "" || strings.ContainsAny(value, " \t\n\r|;&`$<>") || strings.Contains(value, "@") || strings.HasPrefix(value, "file:") || strings.HasPrefix(value, "~") || strings.HasPrefix(value, "$HOME") || strings.HasPrefix(value, "${HOME}") || strings.HasPrefix(value, "$XDG_") || strings.HasPrefix(value, "${XDG_") {
		return "requested url"
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		return "requested url"
	}
	return limitTextBytes(value, maxDisplayTextBytes)
}

func safeFetchPreviewLines(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	const maxPreviewLines = 12
	limit := len(lines)
	if limit > maxPreviewLines {
		limit = maxPreviewLines
	}
	items := make([]string, 0, limit)
	for _, line := range lines[:limit] {
		items = append(items, safeCommandOutputLine(line))
	}
	return items
}

func semanticReadLineRange(lineRange ReadLineRangeView) SemanticReadLineRange {
	return SemanticReadLineRange(lineRange)
}

func hasReadRange(lineRange ReadLineRangeView) bool {
	return lineRange.StartLine > 0 || lineRange.EndLine > 0 || lineRange.Limit > 0
}

func safePreviewLines(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	const maxPreviewLines = 12
	limit := len(lines)
	if limit > maxPreviewLines {
		limit = maxPreviewLines
	}
	items := make([]string, 0, limit)
	for _, line := range lines[:limit] {
		items = append(items, safeText(line))
	}
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

func safeReadTargetPath(value string) string {
	value = strings.Join(strings.Fields(stripTerminalControls(value)), " ")
	if value == "" {
		return "requested path"
	}
	if secretLikeText.MatchString(value) {
		return "[redacted]"
	}
	slashPath := strings.ReplaceAll(value, "\\", "/")
	if strings.HasPrefix(slashPath, "/") || strings.HasPrefix(slashPath, "~") || strings.HasPrefix(slashPath, "$HOME") || strings.HasPrefix(slashPath, "${HOME}") || strings.HasPrefix(slashPath, "$XDG_") || strings.HasPrefix(slashPath, "${XDG_") || (strings.Contains(slashPath, "../") || strings.Contains(slashPath, "/..")) || strings.Contains(slashPath, "\x2eaila") || strings.Contains(slashPath, "\x2eagentera") || strings.Contains(slashPath, "\x2econfig") {
		return "[path-redacted]"
	}
	return limitTextBytes(value, maxDisplayTextBytes)
}

func safeSearchTarget(value string) string {
	if value == "" {
		return ""
	}
	return safeReadTargetPath(value)
}

func semanticFocus(state ViewState) string {
	if state.ModelSwitch != nil && state.ModelSwitch.Focus {
		return "model_switch"
	}
	if state.AutonomySwitch != nil && state.AutonomySwitch.Focus {
		return "autonomy_switch"
	}
	if state.FileReference != nil && state.FileReference.Focus {
		return "file_reference"
	}
	if state.Session != nil && state.Session.Focus {
		return "session"
	}
	if historyVisible(state) && state.HistoryFocus {
		return "history"
	}
	if diffVisible(state) && state.DiffFocus {
		return "diff"
	}
	return "prompt"
}

func stripTerminalControls(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); {
		switch value[i] {
		case 0x90, 0x9e, 0x9f:
			i = skipUntilStringTerminator(value, i+1)
			continue
		case 0x9d:
			i = skipUntilBELOrStringTerminator(value, i+1)
			continue
		}
		r, size := utf8.DecodeRuneInString(value[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		if r == '\x1b' {
			i += size
			i = skipEscapeSequence(value, i)
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

func skipEscapeSequence(value string, index int) int {
	if index >= len(value) {
		return index
	}
	switch value[index] {
	case '[':
		return skipUntilFinalByte(value, index+1)
	case ']':
		return skipUntilBELOrStringTerminator(value, index+1)
	case 'P', '^', '_':
		return skipUntilStringTerminator(value, index+1)
	default:
		_, size := utf8.DecodeRuneInString(value[index:])
		return index + size
	}
}

func skipUntilFinalByte(value string, index int) int {
	for index < len(value) {
		r, size := utf8.DecodeRuneInString(value[index:])
		index += size
		if r >= 0x40 && r <= 0x7e {
			break
		}
	}
	return index
}

func skipUntilBELOrStringTerminator(value string, index int) int {
	for index < len(value) {
		r, size := utf8.DecodeRuneInString(value[index:])
		index += size
		if r == '\a' {
			break
		}
		if r == '\x1b' && index < len(value) && value[index] == '\\' {
			index++
			break
		}
	}
	return index
}

func skipUntilStringTerminator(value string, index int) int {
	for index < len(value) {
		r, size := utf8.DecodeRuneInString(value[index:])
		index += size
		if r == '\x1b' && index < len(value) && value[index] == '\\' {
			index++
			break
		}
	}
	return index
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

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
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

type diffRow struct {
	Kind    string
	Path    string
	Text    string
	OldLine int
	NewLine int
}

func diffSurfaceLines(state ViewState) []string {
	if !diffVisible(state) {
		return nil
	}
	diff := state.Diff
	if diff == nil {
		diff = &DiffView{Source: "app.diff", Status: "empty", Empty: true}
	}
	selected := clampDiffSelection(state)
	rows := diffRows(state)
	lines := []string{
		"read-only: true",
		"source: " + safeText(diff.Source),
		"status: " + safeText(defaultString(diff.Status, "ready")),
		fmt.Sprintf("files: %d", len(diff.Files)),
		fmt.Sprintf("selected: %d", selected+1),
	}
	if diff.ErrorMessage != "" {
		lines = append(lines, "error: "+safeText(diff.ErrorMessage))
	}
	if diff.Empty || len(diff.Files) == 0 {
		lines = append(lines, "no changes")
		return lines
	}
	start := diffWindowStart(state, 14)
	for index, row := range visibleDiffRows(state, 14) {
		marker := " "
		absolute := start + index
		if absolute == selected {
			marker = ">"
		}
		lines = append(lines, marker+" "+row.Text)
	}
	if selected >= 0 && selected < len(rows) {
		row := rows[selected]
		lines = append(lines,
			"selected kind: "+safeText(row.Kind),
			"selected path: "+safeText(row.Path),
			"selected text: "+safeText(row.Text),
		)
	}
	return lines
}

func diffVisible(state ViewState) bool {
	return state.SurfaceTitle == "diff" || state.CommandRoute == "diff" || state.DiffFocus || state.Diff != nil
}

func diffRows(state ViewState) []diffRow {
	if state.Diff == nil {
		return nil
	}
	rows := make([]diffRow, 0, len(state.Diff.Files)*4)
	for _, file := range state.Diff.Files {
		path := safeDecisionTarget(file.Path)
		status := safeText(file.Status)
		if status == "" {
			status = "modified"
		}
		rows = append(rows, diffRow{Kind: "file", Path: path, Text: "file: " + path + " status: " + status})
		for _, hunk := range file.Hunks {
			header := safeText(hunk.Header)
			rows = append(rows, diffRow{Kind: "hunk", Path: path, Text: "hunk: " + header})
			for _, line := range hunk.Lines {
				prefix := " "
				switch line.Kind {
				case "addition":
					prefix = "+"
				case "removal":
					prefix = "-"
				}
				rows = append(rows, diffRow{Kind: safeText(line.Kind), Path: path, Text: prefix + " " + safeText(line.Text), OldLine: line.OldLine, NewLine: line.NewLine})
			}
		}
	}
	return rows
}

func clampDiffSelection(state ViewState) int {
	rows := diffRows(state)
	if len(rows) == 0 {
		return 0
	}
	if state.DiffSelected < 0 {
		return 0
	}
	if state.DiffSelected >= len(rows) {
		return len(rows) - 1
	}
	return state.DiffSelected
}

func diffWindowStart(state ViewState, window int) int {
	rows := diffRows(state)
	selected := clampDiffSelection(state)
	if len(rows) <= window || window <= 0 {
		return 0
	}
	start := selected - window/2
	if start < 0 {
		return 0
	}
	maxStart := len(rows) - window
	if start > maxStart {
		return maxStart
	}
	return start
}

func visibleDiffRows(state ViewState, window int) []diffRow {
	rows := diffRows(state)
	if window <= 0 || len(rows) <= window {
		return rows
	}
	start := diffWindowStart(state, window)
	end := start + window
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}

func semanticDiff(state ViewState) *SemanticDiff {
	if !diffVisible(state) {
		return nil
	}
	diff := state.Diff
	if diff == nil {
		diff = &DiffView{Source: "app.diff", Status: "empty", Empty: true}
	}
	files := make([]SemanticDiffFile, 0, len(diff.Files))
	for _, file := range diff.Files {
		semanticFile := SemanticDiffFile{Path: safeDecisionTarget(file.Path), OldPath: safeDecisionTarget(file.OldPath), Status: safeText(file.Status)}
		for _, hunk := range file.Hunks {
			semanticHunk := SemanticDiffHunk{Header: safeText(hunk.Header), OldStart: hunk.OldStart, OldLines: hunk.OldLines, NewStart: hunk.NewStart, NewLines: hunk.NewLines}
			for _, line := range hunk.Lines {
				semanticHunk.Lines = append(semanticHunk.Lines, SemanticDiffLine{Kind: safeText(line.Kind), Text: safeText(line.Text), OldLine: line.OldLine, NewLine: line.NewLine})
			}
			semanticFile.Hunks = append(semanticFile.Hunks, semanticHunk)
		}
		files = append(files, semanticFile)
	}
	rows := diffRows(state)
	selected := clampDiffSelection(state)
	selectedLine := ""
	if selected >= 0 && selected < len(rows) {
		selectedLine = rows[selected].Text
	}
	return &SemanticDiff{Visible: true, ReadOnly: true, Source: safeText(diff.Source), Status: safeText(defaultString(diff.Status, "ready")), Focus: state.DiffFocus, Empty: diff.Empty || len(diff.Files) == 0, ErrorMessage: safeText(diff.ErrorMessage), FileCount: len(diff.Files), SelectedIndex: selected, SelectedLine: selectedLine, Files: files}
}

func semanticDiffRegionItems(state ViewState) []string {
	semantic := semanticDiff(state)
	if semantic == nil {
		return nil
	}
	items := []string{
		"read_only: true",
		"source: " + semantic.Source,
		"status: " + semantic.Status,
		"focus: " + boolLabel(semantic.Focus),
		fmt.Sprintf("file_count: %d", semantic.FileCount),
		fmt.Sprintf("selected_index: %d", semantic.SelectedIndex),
	}
	if semantic.Empty {
		items = append(items, "empty: true")
	}
	if semantic.ErrorMessage != "" {
		items = append(items, "error: "+semantic.ErrorMessage)
	}
	for _, file := range semantic.Files {
		items = append(items, "file: "+file.Path, "file_status: "+file.Status)
		for _, hunk := range file.Hunks {
			items = append(items, "hunk: "+hunk.Header)
			for _, line := range hunk.Lines {
				items = append(items, "line_"+line.Kind+": "+line.Text)
			}
		}
	}
	items = append(items, "app-owned", "display-only")
	return items
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
	if historyUndoEnabled(state.HistoryItems) {
		lines = append(lines, "undo enabled: true")
	}
	start := historyWindowStart(state, 12)
	for index, item := range visibleHistoryItems(state, 12) {
		marker := " "
		absolute := start + index
		if absolute == selected {
			marker = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %s %s %s %s %s", marker, safeText(item.RunID), safeText(item.SessionID), safeText(item.EventID), safeText(item.Kind), historyRowSummary(item)))
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
	lines = append(lines, selectedHistoryMutationLines(item)...)
	return lines
}

func historyRowSummary(item HistoryItem) string {
	if item.Recovery != nil {
		paths := strings.Join(item.Recovery.Paths, ",")
		if paths == "" {
			paths = safeText(item.DisplayText)
		}
		return safeText(fmt.Sprintf("%s %s %s %s", item.Recovery.Command, item.Recovery.Status, item.Recovery.Action, paths))
	}
	if item.Mutation == nil {
		return safeText(item.DisplayText)
	}
	paths := strings.Join(item.Mutation.ChangedPaths, ",")
	if paths == "" {
		paths = safeText(item.DisplayText)
	}
	return safeText(fmt.Sprintf("%s %s %s", item.Mutation.Name, item.Mutation.Status, paths))
}

func selectedHistoryMutationLines(item HistoryItem) []string {
	if item.Recovery != nil {
		recovery := item.Recovery
		lines := []string{
			"selected recovery command: " + safeText(recovery.Command),
			"selected recovery status: " + safeText(recovery.Status),
			"selected recovery target event id: " + safeText(recovery.TargetEventID),
			"selected recovery action: " + safeText(recovery.Action),
			"selected recovery paths: " + safeText(strings.Join(recovery.Paths, ", ")),
			"selected redo available: " + boolLabel(recovery.RedoAvailable),
		}
		if recovery.RedoAction != "" {
			lines = append(lines, "selected redo action: "+safeText(recovery.RedoAction))
		}
		if recovery.PreviousVersion != "" {
			lines = append(lines, "selected previous version: "+safeText(recovery.PreviousVersion))
		}
		if recovery.NewVersion != "" {
			lines = append(lines, "selected new version: "+safeText(recovery.NewVersion))
		}
		if recovery.Reason != "" {
			lines = append(lines, "selected recovery reason: "+safeText(recovery.Reason))
		}
		if recovery.ErrorKind != "" {
			lines = append(lines, "selected error kind: "+safeText(recovery.ErrorKind))
		}
		return lines
	}
	if item.Mutation == nil {
		return nil
	}
	mutation := item.Mutation
	lines := []string{
		"selected mutation tool: " + safeText(mutation.Name),
		"selected mutation status: " + safeText(mutation.Status),
		"selected command source: " + safeText(mutation.CommandSource),
		"selected changed paths: " + safeText(strings.Join(mutation.ChangedPaths, ", ")),
	}
	if mutation.ApprovalID != "" {
		lines = append(lines, "selected approval id: "+safeText(mutation.ApprovalID))
	}
	if mutation.ApprovalAction != "" {
		lines = append(lines, "selected approval action: "+safeText(mutation.ApprovalAction))
	}
	if mutation.ExpectedEffect != "" {
		lines = append(lines, "selected expected effect: "+safeText(mutation.ExpectedEffect))
	}
	if mutation.PreviousVersion != "" {
		lines = append(lines, "selected previous version: "+safeText(mutation.PreviousVersion))
	}
	if mutation.NewVersion != "" {
		lines = append(lines, "selected new version: "+safeText(mutation.NewVersion))
	}
	if mutation.ErrorKind != "" {
		lines = append(lines, "selected error kind: "+safeText(mutation.ErrorKind))
	}
	if item.Undo != nil {
		lines = append(lines,
			"selected undo available: "+boolLabel(item.Undo.Available),
			"selected undo action: "+safeText(item.Undo.Action),
		)
		if item.Undo.Reason != "" {
			lines = append(lines, "selected undo reason: "+safeText(item.Undo.Reason))
		}
	}
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
			Mutation:    semanticHistoryMutation(item.Mutation),
			Undo:        semanticHistoryUndo(item.Undo),
			Recovery:    semanticHistoryRecovery(item.Recovery),
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
		UndoEnabled:   historyUndoEnabled(state.HistoryItems),
		RedoEnabled:   historyRedoEnabled(state.HistoryItems),
		Focus:         state.HistoryFocus,
		Empty:         state.HistoryEmpty || len(state.HistoryItems) == 0,
		Count:         len(state.HistoryItems),
		SelectedIndex: selected,
		SelectedID:    selectedID,
		Items:         items,
	}
}

func semanticHistoryMutation(mutation *HistoryMutationItem) *SemanticHistoryMutation {
	if mutation == nil {
		return nil
	}
	return &SemanticHistoryMutation{
		Name:           safeText(mutation.Name),
		Status:         safeText(mutation.Status),
		CommandSource:  safeText(mutation.CommandSource),
		RequestID:      safeText(mutation.RequestID),
		ApprovalID:     safeText(mutation.ApprovalID),
		ApprovalAction: safeText(mutation.ApprovalAction),
		ChangedPaths:   safeTextSlice(mutation.ChangedPaths),
		RequestedPath:  safeText(mutation.RequestedPath),
		ExpectedEffect: safeText(mutation.ExpectedEffect),
		ErrorKind:      safeText(mutation.ErrorKind),
		ErrorMessage:   safeText(mutation.ErrorMessage),
	}
}

func semanticHistoryUndo(undo *HistoryUndoItem) *SemanticHistoryUndo {
	if undo == nil {
		return nil
	}
	return &SemanticHistoryUndo{
		Available:       undo.Available,
		Action:          safeText(undo.Action),
		Paths:           safeTextSlice(undo.Paths),
		PreviousVersion: safeText(undo.PreviousVersion),
		NewVersion:      safeText(undo.NewVersion),
		Reason:          safeText(undo.Reason),
	}
}

func semanticHistoryRecovery(recovery *HistoryRecoveryItem) *SemanticHistoryRecovery {
	if recovery == nil {
		return nil
	}
	return &SemanticHistoryRecovery{
		Command:            safeText(recovery.Command),
		Status:             safeText(recovery.Status),
		TargetEventID:      safeText(recovery.TargetEventID),
		Action:             safeText(recovery.Action),
		Paths:              safeTextSlice(recovery.Paths),
		PreviousVersion:    safeText(recovery.PreviousVersion),
		NewVersion:         safeText(recovery.NewVersion),
		RedoAvailable:      recovery.RedoAvailable,
		RedoAction:         safeText(recovery.RedoAction),
		Reason:             safeText(recovery.Reason),
		ErrorKind:          safeText(recovery.ErrorKind),
		ErrorMessage:       safeText(recovery.ErrorMessage),
		DecisionRunID:      safeText(recovery.DecisionRunID),
		DecisionCapability: safeText(recovery.DecisionCapability),
	}
}

func historyUndoEnabled(items []HistoryItem) bool {
	for _, item := range items {
		if item.Undo != nil && item.Undo.Available {
			return true
		}
	}
	return false
}

func historyRedoEnabled(items []HistoryItem) bool {
	for _, item := range items {
		if item.Recovery != nil && item.Recovery.RedoAvailable {
			return true
		}
	}
	return false
}

func semanticHistoryRegionItems(state ViewState) []string {
	history := semanticHistory(state)
	if history == nil {
		return nil
	}
	items := []string{
		"read_only: true",
		"undo_enabled: " + boolLabel(history.UndoEnabled),
		"redo_enabled: " + boolLabel(history.RedoEnabled),
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
		if item.Mutation != nil {
			items = append(items,
				"item_mutation: "+item.EventID+" "+item.Mutation.Name+" "+item.Mutation.Status,
				"item_changed_paths: "+strings.Join(item.Mutation.ChangedPaths, ","),
			)
			if item.Mutation.ApprovalID != "" {
				items = append(items, "item_approval_id: "+item.Mutation.ApprovalID)
			}
		}
		if item.Undo != nil {
			items = append(items, "item_undo_available: "+boolLabel(item.Undo.Available))
			if item.Undo.Action != "" {
				items = append(items, "item_undo_action: "+item.Undo.Action)
			}
		}
		if item.Recovery != nil {
			items = append(items,
				"item_recovery: "+item.EventID+" "+item.Recovery.Command+" "+item.Recovery.Status,
				"item_recovery_target: "+item.Recovery.TargetEventID,
				"item_recovery_action: "+item.Recovery.Action,
				"item_recovery_paths: "+strings.Join(item.Recovery.Paths, ","),
				"item_redo_available: "+boolLabel(item.Recovery.RedoAvailable),
			)
			if item.Recovery.RedoAction != "" {
				items = append(items, "item_redo_action: "+item.Recovery.RedoAction)
			}
		}
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
	if state.Search != nil {
		items = append(items, semanticSearchItems(state.Search)...)
	}
	if state.Command != nil {
		items = append(items, semanticBashItems(state.Command)...)
	}
	if state.Utility != nil {
		items = append(items, semanticUtilityItems(state.Utility)...)
	}
	if state.PolicyRoute != nil {
		items = append(items, semanticPolicyRouteItems(state.PolicyRoute)...)
	}
	if state.Context != nil {
		items = append(items, semanticContextItems(state.Context)...)
	}
	if state.Mutation != nil {
		items = append(items, semanticMutationItems(state.Mutation)...)
	}
	if diffVisible(state) {
		items = append(items, semanticDiffRegionItems(state)...)
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

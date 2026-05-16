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
	Fetch              *FetchView
	Mutation           *MutationView
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
	Diff               *DiffView
	DiffSelected       int
	DiffFocus          bool
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
	Mutation    *HistoryMutationItem
	Undo        *HistoryUndoItem
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
	if state.SurfaceTitle == "agent evidence" {
		items = append(items, diagnosticLines(state.Diagnostics)...)
		items = append(items, chatLines(state.Transcript)...)
		items = append(items, approvalLines(state.Approval)...)
		items = append(items, readLines(state.Read)...)
		items = append(items, searchLines(state.Search)...)
		items = append(items, commandLines(state.Command)...)
		items = append(items, fetchLines(state.Fetch)...)
		items = append(items, mutationLines(state.Mutation)...)
		items = append(items, memoryLines(state)...)
		items = append(items, queueLines(state)...)
		items = append(items, surfaceLines(state.CommandRoute, state.RouteSource, state.SurfaceTitle, state.SurfaceLines)...)
		return items
	}
	if state.SurfaceTitle != "" {
		items = append(items, diagnosticLines(state.Diagnostics)...)
	}
	items = append(items, approvalLines(state.Approval)...)
	items = append(items, readLines(state.Read)...)
	items = append(items, searchLines(state.Search)...)
	items = append(items, commandLines(state.Command)...)
	items = append(items, fetchLines(state.Fetch)...)
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
	for _, code := range []string{ansiBold, ansiDim, ansiCyan, ansiGreen, ansiRed, ansiYellow, ansiReset} {
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
	Diff        *SemanticDiff        `json:"diff,omitempty"`
	Read        *SemanticRead        `json:"read_tool,omitempty"`
	Search      *SemanticSearch      `json:"search_tool,omitempty"`
	Bash        *SemanticBash        `json:"bash_tool,omitempty"`
	Fetch       *SemanticFetch       `json:"fetch_tool,omitempty"`
	Mutation    *SemanticMutation    `json:"mutation_tool,omitempty"`
	Approval    *SemanticApproval    `json:"approval,omitempty"`
	Regions     []SemanticRegion     `json:"regions"`
	Actions     []SemanticAction     `json:"actions"`
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
	EventID     string                   `json:"event_id"`
	RunID       string                   `json:"run_id"`
	SessionID   string                   `json:"session_id"`
	Kind        string                   `json:"kind"`
	Source      string                   `json:"source"`
	Provenance  string                   `json:"provenance"`
	DisplayText string                   `json:"display_text"`
	Mutation    *SemanticHistoryMutation `json:"mutation,omitempty"`
	Undo        *SemanticHistoryUndo     `json:"undo,omitempty"`
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
	if state.Fetch != nil {
		regions = append(regions, SemanticRegion{Name: "fetch_tool", Visible: true, Items: semanticFetchItems(state.Fetch)})
	}
	if state.Mutation != nil {
		regions = append(regions, SemanticRegion{Name: "mutation_tool", Visible: true, Items: semanticMutationItems(state.Mutation)})
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
		SemanticRegion{Name: "prompt", Visible: true, Items: []string{promptLine(state.PromptInput)}},
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
		Diff:        semanticDiff(state),
		Read:        semanticRead(state.Read),
		Search:      semanticSearch(state.Search),
		Bash:        semanticBash(state.Command),
		Fetch:       semanticFetch(state.Fetch),
		Mutation:    semanticMutation(state.Mutation),
		Approval:    semanticApproval(state.Approval),
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

func historyUndoEnabled(items []HistoryItem) bool {
	for _, item := range items {
		if item.Undo != nil && item.Undo.Available {
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

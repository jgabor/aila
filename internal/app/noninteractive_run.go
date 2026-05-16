package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tools"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

const (
	nonInteractiveRunMode           = "non_interactive_read_only"
	nonInteractiveRunPromptMaxBytes = 512
	nonInteractiveRunTextMaxBytes   = 240
)

// NonInteractiveRunRequest describes one bounded read-only CLI run request.
type NonInteractiveRunRequest struct {
	Version       string
	Prompt        string
	WorkspacePath string
}

type nonInteractiveRunReport struct {
	Version        string
	Prompt         string
	Status         string
	InspectedFiles []tui.RunMemoryFileView
	Commands       []tui.RunMemoryCommandView
	Blockers       []string
	Caveats        []string
	SourceRefs     []string
	StoredSession  bool
	StoredHistory  bool
}

// NonInteractiveRunCommandOutput runs a bounded read-only non-interactive task and returns stable CLI output.
func NonInteractiveRunCommandOutput(ctx context.Context, request NonInteractiveRunRequest) (string, error) {
	report, err := runNonInteractiveReadOnly(ctx, request)
	if err != nil {
		return "", err
	}
	return formatNonInteractiveRunReport(report), nil
}

func runNonInteractiveReadOnly(ctx context.Context, request NonInteractiveRunRequest) (nonInteractiveRunReport, error) {
	if err := ctx.Err(); err != nil {
		return nonInteractiveRunReport{}, fmt.Errorf("run non-interactive read-only task: %w", err)
	}
	workspace := request.WorkspacePath
	if strings.TrimSpace(workspace) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nonInteractiveRunReport{}, fmt.Errorf("resolve run workspace: %w", err)
		}
		workspace = cwd
	}
	workspace = filepath.Clean(workspace)

	report := nonInteractiveRunReport{
		Version: strings.TrimSpace(request.Version),
		Prompt:  boundRunText(request.Prompt, nonInteractiveRunPromptMaxBytes),
		Status:  "completed",
		Caveats: []string{"deterministic read-only run; provider model execution deferred"},
	}
	if report.Version == "" {
		report.Version = "dev"
	}
	if strings.TrimSpace(report.Prompt) == "" {
		report.Status = "blocked"
		report.Blockers = append(report.Blockers, "prompt is required")
		return report, nil
	}

	inspectKnownRunFiles(ctx, workspace, &report)
	runFixedRunChecks(ctx, workspace, &report)
	if len(report.InspectedFiles) == 0 {
		report.Blockers = append(report.Blockers, "no known repo files were inspected")
	}
	report.Status = runReportStatus(report)
	persistNonInteractiveRun(ctx, workspace, &report)
	return report, nil
}

func inspectKnownRunFiles(ctx context.Context, workspace string, report *nonInteractiveRunReport) {
	for _, candidate := range []string{"README.md", "ROADMAP.md", "AGENTS.md"} {
		decision := permission.DecideRecord(permission.AutonomyRead, permission.NewReadOperation(candidate))
		if !decision.Allowed {
			report.Blockers = append(report.Blockers, "read denied for "+candidate+": "+decision.Reason)
			continue
		}
		validated, validationErr := tools.ValidateReadRequest(workspace, tools.ReadRequest{
			Path:            candidate,
			LineLimit:       40,
			MaxPreviewBytes: 4096,
			Source: tools.ReadSourceMetadata{
				Caller:      "noninteractive.run",
				RequestID:   "run-read-" + strings.TrimSuffix(candidate, filepath.Ext(candidate)),
				Description: "inspect repository context for non-interactive run",
			},
		})
		if validationErr.Kind != "" {
			report.Caveats = append(report.Caveats, candidate+" not inspected: "+validationErr.Message)
			continue
		}
		result := tools.ExecuteRead(ctx, validated)
		if result.Error.Kind != "" && result.Error.Kind != tools.ReadErrorNone {
			report.Caveats = append(report.Caveats, candidate+" not inspected: "+result.Error.Message)
			continue
		}
		rangeRef := fmt.Sprintf("%s:%d-%d", result.WorkspaceRelativePath, result.EffectiveRange.StartLine, result.EffectiveRange.EndLine)
		report.InspectedFiles = append(report.InspectedFiles, tui.RunMemoryFileView{
			Path:      result.WorkspaceRelativePath,
			Status:    "completed",
			LineStart: result.EffectiveRange.StartLine,
			LineEnd:   result.EffectiveRange.EndLine,
			SourceRef: rangeRef,
		})
		report.SourceRefs = appendUniqueString(report.SourceRefs, rangeRef)
	}
}

func runFixedRunChecks(ctx context.Context, workspace string, report *nonInteractiveRunReport) {
	checks := [][]string{{"git", "status", "--short", "--branch"}, {"git", "diff", "--stat"}}
	for index, argv := range checks {
		validated, validationErr := tools.ValidateBashRequest(workspace, tools.BashRequest{
			Argv:           argv,
			WorkingDir:     ".",
			MaxOutputBytes: 2048,
			TimeoutMillis:  3000,
			Source: tools.BashSourceMetadata{
				Caller:      "noninteractive.run",
				RequestID:   fmt.Sprintf("run-check-%d", index+1),
				Description: "inspect repository state for non-interactive run",
			},
		})
		command := strings.Join(argv, " ")
		if validationErr.Kind != "" {
			report.Blockers = append(report.Blockers, command+" rejected: "+validationErr.Message)
			continue
		}
		operation := permission.NewBashInspectionOperation(validated.EffectiveArgv, validated.WorkspaceRelativeWorkDir, validated.ExpectedEffect)
		decision := permission.DecideRecord(permission.AutonomyRead, operation)
		if !decision.Allowed {
			report.Blockers = append(report.Blockers, command+" denied: "+decision.Reason)
			continue
		}
		result := tools.ExecuteBash(ctx, validated)
		summary := commandSummary(result)
		report.Commands = append(report.Commands, tui.RunMemoryCommandView{
			Command:  command,
			Status:   result.Status,
			ExitCode: result.ExitCode,
			Summary:  summary,
		})
		report.SourceRefs = appendUniqueString(report.SourceRefs, command)
		if result.Error.Kind != "" && result.Error.Kind != tools.BashErrorNone {
			report.Caveats = append(report.Caveats, command+" "+result.Status+": "+result.Error.Message)
		}
	}
}

func persistNonInteractiveRun(ctx context.Context, workspace string, report *nonInteractiveRunReport) {
	store, err := state.OpenProjectStore(ctx, workspace)
	if err != nil {
		report.Caveats = append(report.Caveats, "session state not stored: "+boundedStoreError(err))
		report.Status = runReportStatus(*report)
		return
	}
	if appendNonInteractiveRunHistory(ctx, store, *report) {
		report.StoredHistory = true
	} else {
		report.Caveats = append(report.Caveats, "history state not stored")
	}
	report.Status = runReportStatus(*report)
	report.StoredSession = true
	view := nonInteractiveRunView(*report)
	if _, err := store.WriteCurrentSessionSnapshot(ctx, NewCurrentSessionSnapshot(view)); err != nil {
		report.StoredSession = false
		report.Caveats = append(report.Caveats, "session state not stored: "+boundedStoreError(err))
		report.Status = runReportStatus(*report)
	}
}

func nonInteractiveRunView(report nonInteractiveRunReport) tui.ViewState {
	view := tui.IdleEmptyState()
	view.Phase = workflow.PhaseIdle.DisplayLabel()
	view.PhaseSource = workflow.PhaseIdle.String()
	view.RuntimeStatus = "idle"
	view.StatusSource = "noninteractive.run"
	view.StatusDetail = "read-only run " + report.Status
	view.RuntimeResult = runAssistantSummary(report)
	view.Transcript = []tui.TranscriptTurn{{UserText: report.Prompt}, {AssistantText: view.RuntimeResult}}
	view.RunMemory = runMemoryFromReport(report)
	return view
}

func appendNonInteractiveRunHistory(ctx context.Context, store state.Store, report nonInteractiveRunReport) bool {
	read, err := store.ReadFakeHistory(ctx)
	if err != nil || read.State == state.FakeHistoryRecoveryNeeded {
		return false
	}
	start := len(read.Events) + 1
	events := []history.FakeEvent{
		nonInteractiveRunHistoryEvent(start, history.EventKindPrompt, "run.prompt", "user", "noninteractive run prompt "+report.Prompt),
		nonInteractiveRunHistoryEvent(start+1, history.EventKindResponse, "run.response", "noninteractive.run", runAssistantSummary(report)),
		nonInteractiveRunHistoryEvent(start+2, history.EventKindRuntime, "run.complete", "noninteractive.run", "noninteractive run "+report.Status+" inspected="+fmt.Sprint(len(report.InspectedFiles))+" commands="+fmt.Sprint(len(report.Commands))),
	}
	for _, command := range report.Commands {
		events = append(events, nonInteractiveRunHistoryEvent(start+len(events), history.EventKindCommand, "run.check", "noninteractive.run", "check "+command.Command+" "+command.Status))
	}
	for _, event := range events {
		result, err := store.AppendFakeHistory(ctx, event)
		if err != nil || result.State == state.FakeHistoryRecoveryNeeded {
			return false
		}
	}
	return true
}

func nonInteractiveRunHistoryEvent(number int, kind history.EventKind, provenance string, source string, displayText string) history.FakeEvent {
	return history.FakeEvent{
		SchemaVersion: history.FakeEventSchemaVersion,
		Kind:          kind,
		EventID:       fmt.Sprintf("noninteractive-run-%d", number),
		RunID:         "noninteractive-run",
		SessionID:     currentSessionID,
		Source:        source,
		Provenance:    provenance,
		DisplayText:   boundRunText(displayText, nonInteractiveRunTextMaxBytes),
	}
}

func runMemoryFromReport(report nonInteractiveRunReport) *tui.RunMemoryView {
	return &tui.RunMemoryView{
		Mode:           nonInteractiveRunMode,
		Prompt:         report.Prompt,
		Status:         report.Status,
		InspectedFiles: append([]tui.RunMemoryFileView(nil), report.InspectedFiles...),
		Commands:       append([]tui.RunMemoryCommandView(nil), report.Commands...),
		Blockers:       append([]string(nil), report.Blockers...),
		Caveats:        append([]string(nil), report.Caveats...),
		SourceRefs:     append([]string(nil), report.SourceRefs...),
		StoredSession:  report.StoredSession,
		StoredHistory:  report.StoredHistory,
	}
}

func formatNonInteractiveRunReport(report nonInteractiveRunReport) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "aila %s\n", report.Version)
	fmt.Fprintln(&builder, "command: run")
	fmt.Fprintln(&builder, "mode: "+nonInteractiveRunMode)
	fmt.Fprintln(&builder, "status: "+report.Status)
	fmt.Fprintln(&builder, "prompt: "+safeRunLine(report.Prompt))
	fmt.Fprintln(&builder, "inspected_files:")
	if len(report.InspectedFiles) == 0 {
		fmt.Fprintln(&builder, "- none")
	} else {
		for _, file := range report.InspectedFiles {
			fmt.Fprintf(&builder, "- %s status=%s source_ref=%s\n", safeRunLine(file.Path), safeRunLine(file.Status), safeRunLine(file.SourceRef))
		}
	}
	fmt.Fprintln(&builder, "commands_run:")
	if len(report.Commands) == 0 {
		fmt.Fprintln(&builder, "- none")
	} else {
		for _, command := range report.Commands {
			fmt.Fprintf(&builder, "- %s status=%s exit=%d summary=%s\n", safeRunLine(command.Command), safeRunLine(command.Status), command.ExitCode, safeRunLine(command.Summary))
		}
	}
	appendRunList(&builder, "blockers", report.Blockers)
	appendRunList(&builder, "caveats", report.Caveats)
	appendRunList(&builder, "source_refs", report.SourceRefs)
	fmt.Fprintf(&builder, "stored_session: %t\n", report.StoredSession)
	fmt.Fprintf(&builder, "stored_history: %t\n", report.StoredHistory)
	return builder.String()
}

func appendRunList(builder *strings.Builder, label string, values []string) {
	fmt.Fprintln(builder, label+":")
	if len(values) == 0 {
		fmt.Fprintln(builder, "- none")
		return
	}
	for _, value := range values {
		fmt.Fprintln(builder, "- "+safeRunLine(value))
	}
}

func runAssistantSummary(report nonInteractiveRunReport) string {
	return fmt.Sprintf("Read-only run %s: inspected %d file(s), ran %d check(s).", report.Status, len(report.InspectedFiles), len(report.Commands))
}

func runReportStatus(report nonInteractiveRunReport) string {
	if len(report.Blockers) > 0 {
		return "blocked"
	}
	if len(report.Caveats) > 0 {
		return "flagged"
	}
	return "completed"
}

func commandSummary(result tools.BashResult) string {
	for _, text := range []string{result.Stdout.Text, result.Stderr.Text} {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				return boundRunText(line, nonInteractiveRunTextMaxBytes)
			}
		}
	}
	if result.Status == "completed" {
		return "no output"
	}
	if result.Error.Message != "" {
		return result.Error.Message
	}
	return result.Status
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func safeRunLine(value string) string {
	return strings.ReplaceAll(boundRunText(value, nonInteractiveRunTextMaxBytes), "\n", " ")
}

func boundRunText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	if limit <= len("[truncated]") {
		return value[:limit]
	}
	return strings.TrimSpace(value[:limit-len(" [truncated]")]) + " [truncated]"
}

package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

const documentDefaultTargetPath = "docs/aila-documentation-output.md"

type documentArtifactPersistence struct {
	Path       string
	Status     string
	Diagnostic *diagnostic.Diagnostic
}

func (controller *sessionController) openDocumentView() []tui.DiagnosticView {
	writeTurn := controller.runner.proposeWriteTool(documentMutationRequest())
	mutation := controller.runner.model.LastMutation
	diagnostics := controller.persistMutationHistory(writeTurn)
	request := documentRequestFromMutation(mutation, workflowPhaseFromView(controller.view))
	turn := controller.runner.proposeCapability(request)
	if writeTurn.Mutation != nil {
		turn.Mutation = writeTurn.Mutation
	}
	if writeTurn.Approval != nil {
		turn.Approval = writeTurn.Approval
	}
	if writeTurn.ApprovalDecision != nil {
		turn.ApprovalDecision = writeTurn.ApprovalDecision
	}
	persistence := controller.persistDocumentPayload(controller.runner.model.LastCapability)
	turn.Document = documentView(controller.runner.model.LastCapability, request.Phase, persistence)
	if turn.Document != nil {
		turn.StatusDetail = "document capability status"
	}
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	if persistence.Diagnostic != nil {
		diagnostics = append(diagnostics, diagnosticViews([]diagnostic.Diagnostic{*persistence.Diagnostic})...)
	}
	return diagnostics
}

func documentMutationRequest() runtime.MutationToolRequest {
	content := documentTargetContent()
	return runtime.MutationToolRequest{
		ToolName:       runtime.MutationToolWrite,
		Path:           documentDefaultTargetPath,
		TargetVersion:  "missing",
		Content:        content,
		ExpectedEffect: "create bounded documentation alignment output",
		Source: runtime.MutationSourceMetadata{
			Caller:      string(capability.NameDocument),
			RequestID:   "document-alignment",
			Description: "documentation alignment capability write step",
		},
	}
}

func documentTargetContent() string {
	return strings.Join([]string{
		"# Aila Documentation Alignment",
		"",
		"Capability: document",
		"Target behavior: /document routes documentation writes through mutation safety.",
		"",
		"Plan:",
		"- Identify source behavior from app-supplied evidence.",
		"- Write the bounded documentation target through the mutation tool.",
		"- Persist the documentation alignment artifact through the project store.",
		"",
		"Caveats:",
		"- Deterministic app-supplied documentation evidence only.",
		"",
	}, "\n")
}

func documentRequestFromMutation(mutation runtime.MutationToolResult, phase workflow.Phase) capability.Request {
	path := mutation.WorkspaceRelativePath
	if path == "" {
		path = mutation.RequestedPath
	}
	if path == "" {
		path = documentDefaultTargetPath
	}
	status := defaultString(mutation.Status, "completed")
	metadata := map[string]string{
		capability.DocumentMetadataTargetPath:     path,
		capability.DocumentMetadataTargetTitle:    "Aila documentation alignment",
		capability.DocumentMetadataSourceBehavior: "/document routes documentation writes through mutation safety and records history.",
		capability.DocumentMetadataPlanID:         "document-command-safety",
		capability.DocumentMetadataPlanSummary:    "Record how the document command keeps documentation aligned without bypass writes.",
		capability.DocumentMetadataPlanSteps:      "identify source behavior from app evidence|write docs through mutation tool|persist summary artifact through state store|show diff and caveats in the TUI",
		capability.DocumentMetadataOutputSummary:  "Documented the /document command mutation safety path.",
		capability.DocumentMetadataDiffLines:      "+ # Aila Documentation Alignment|+ Capability: document|+ Target behavior: /document routes documentation writes through mutation safety.",
		capability.DocumentMetadataCaveats:        "deterministic app-supplied documentation evidence only|provider-backed documentation generation deferred",
		capability.DocumentMetadataNextAction:     "Audit the documentation alignment before continuing.",
		capability.DocumentMetadataDurable:        "true",
		capability.DocumentMetadataToolName:       defaultString(mutation.ToolName, string(runtime.MutationToolWrite)),
		capability.DocumentMetadataToolStatus:     status,
		capability.DocumentMetadataExpectedEffect: defaultString(mutation.ExpectedEffect, "create bounded documentation alignment output"),
	}
	if mutation.Decision.Source != "" {
		metadata[capability.DocumentMetadataDecisionSource] = mutation.Decision.Source
	}
	if mutation.Decision.Autonomy != "" {
		metadata[capability.DocumentMetadataDecisionAutonomy] = mutation.Decision.Autonomy
	}
	metadata[capability.DocumentMetadataDecisionAllowed] = strconv.FormatBool(mutation.Decision.Allowed)
	metadata[capability.DocumentMetadataApprovalRequired] = strconv.FormatBool(mutation.Decision.ApprovalRequired)
	metadata[capability.DocumentMetadataBytesWritten] = strconv.Itoa(mutation.BytesWritten)
	if mutation.Error.Kind != "" && mutation.Error.Kind != runtime.MutationToolErrorNone {
		metadata[capability.DocumentMetadataErrorKind] = string(mutation.Error.Kind)
	}
	if mutation.Error.Message != "" {
		metadata[capability.DocumentMetadataErrorMessage] = mutation.Error.Message
	}
	return capability.Request{
		ID:         "command-document",
		Capability: capability.NameDocument,
		Input:      metadata[capability.DocumentMetadataSourceBehavior],
		Phase:      normalizeDocumentPhase(phase),
		SourceRefs: []capability.SourceRef{
			{ID: "document-command", Kind: "command", Command: "/document", Excerpt: "app-owned document command"},
			{ID: "document-workflow-doc", Kind: "doc", Path: "docs/workflow-architecture.md", LineStart: 266, LineEnd: 275, Excerpt: "document is BUILD-owned docs alignment"},
			{ID: "document-mutation-result", Kind: "tool_result", Path: path, Excerpt: documentMutationSummary(mutation)},
		},
		Metadata: metadata,
	}
}

func normalizeDocumentPhase(phase workflow.Phase) workflow.Phase {
	if phase == "" || phase == workflow.PhaseIdle {
		return workflow.PhaseBuild
	}
	return phase
}

func documentMutationSummary(mutation runtime.MutationToolResult) string {
	path := mutation.WorkspaceRelativePath
	if path == "" {
		path = mutation.RequestedPath
	}
	if path == "" {
		path = documentDefaultTargetPath
	}
	if mutation.Error.Message != "" {
		return fmt.Sprintf("documentation write %s on %s: %s", defaultString(mutation.Status, "failed"), path, mutation.Error.Message)
	}
	return fmt.Sprintf("documentation write %s on %s", defaultString(mutation.Status, "completed"), path)
}

func (controller *sessionController) persistDocumentPayload(payload capability.ExitPayload) documentArtifactPersistence {
	if payload.Document == nil || strings.TrimSpace(payload.Document.DocumentArtifact) == "" {
		return documentArtifactPersistence{Status: "not_written"}
	}
	return writeDocumentArtifact(controller.ctx, controller.workspacePath, payload.Document.DocumentArtifact)
}

func writeDocumentArtifact(ctx context.Context, workspacePath string, document string) documentArtifactPersistence {
	store, err := state.OpenProjectStore(ctx, workspacePath)
	if err != nil {
		return documentArtifactPersistence{Status: "recovery_needed", Diagnostic: documentArtifactDiagnostic(fmt.Errorf("open project store: %w", err))}
	}
	artifact, err := store.WriteArtifact(ctx, state.ArtifactDocumentation, state.OwnerApp, []byte(document))
	if err != nil {
		return documentArtifactPersistence{Status: "recovery_needed", Diagnostic: documentArtifactDiagnostic(err)}
	}
	return documentArtifactPersistence{Path: artifact.Path, Status: "written"}
}

func documentArtifactDiagnostic(err error) *diagnostic.Diagnostic {
	message := "document artifact write failed"
	if err != nil {
		message += ": " + boundedStoreError(err)
	}
	diagnostic := diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryState,
		Source:           diagnostic.SourceStateSnapshot,
		Severity:         diagnostic.SeverityWarning,
		Message:          message,
		AffectedArtifact: diagnostic.ArtifactDocumentation,
		RecoveryAction:   diagnostic.RecoveryInspect,
		UserInputNeeded:  true,
	})
	return &diagnostic
}

func documentView(payload capability.ExitPayload, current workflow.Phase, persistence documentArtifactPersistence) *tui.DocumentView {
	if payload.Capability != capability.NameDocument {
		return nil
	}
	recommendation := policy.RecommendCapabilitySuccessor(current, payload)
	artifactStatus := persistence.Status
	if artifactStatus == "" {
		artifactStatus = "available"
	}
	var output capability.DocumentOutput
	if payload.Document != nil {
		output = *payload.Document
	}
	artifactPath := valueOr(output.DocumentArtifactPath, ".aila/artifacts/documentation.md")
	if persistence.Path != "" {
		artifactPath = persistence.Path
	}
	caveats := append([]string(nil), output.Caveats...)
	if len(caveats) == 0 && payload.Document == nil {
		caveats = append([]string(nil), payload.Concerns...)
	}
	return &tui.DocumentView{
		Source:               "app.document",
		Capability:           string(payload.Capability),
		Signal:               string(payload.Signal),
		CurrentPhase:         current.String(),
		Summary:              payload.Summary,
		RecommendedSuccessor: string(payload.RecommendedSuccessor),
		SuccessorValid:       recommendation.SuccessorValid,
		TransitionClaimed:    false,
		DisplayOnly:          true,
		Target:               tui.DocumentTargetView{Path: output.Target.Path, Title: output.Target.Title, SourceBehavior: output.Target.SourceBehavior},
		Plan:                 tui.DocumentPlanView{ID: output.Plan.ID, Summary: output.Plan.Summary, Steps: append([]string(nil), output.Plan.Steps...)},
		OutputSummary:        output.OutputSummary,
		ChangedDocs:          documentChangeViews(output.ChangedDocs),
		DiffLines:            append([]string(nil), output.DiffLines...),
		Mutation: tui.DocumentMutationView{
			Name:             output.Mutation.ToolName,
			Status:           output.Mutation.Status,
			Path:             output.Mutation.Path,
			ExpectedEffect:   output.Mutation.ExpectedEffect,
			DecisionSource:   output.Mutation.DecisionSource,
			DecisionAutonomy: output.Mutation.DecisionAutonomy,
			DecisionAllowed:  output.Mutation.DecisionAllowed,
			ApprovalRequired: output.Mutation.ApprovalRequired,
			BytesWritten:     output.Mutation.BytesWritten,
			ErrorKind:        output.Mutation.ErrorKind,
			ErrorMessage:     output.Mutation.ErrorMessage,
		},
		Caveats:              caveats,
		NeededInput:          payload.NeededInput,
		NextAction:           payload.NextAction,
		DocumentArtifactPath: artifactPath,
		ArtifactStatus:       artifactStatus,
		ArtifactRefs:         documentArtifactRefViews(payload.ArtifactRefs),
		SourceRefs:           documentSourceRefViews(payload.SourceRefs),
		BoundaryRequests:     documentBoundaryRequestViews(payload.BoundaryRequests),
	}
}

func documentChangeViews(changes []capability.DocumentChange) []tui.DocumentChangeView {
	views := make([]tui.DocumentChangeView, 0, len(changes))
	for _, change := range changes {
		views = append(views, tui.DocumentChangeView{Path: change.Path, Status: change.Status, Summary: change.Summary})
	}
	return views
}

func documentArtifactRefViews(refs []capability.ArtifactRef) []tui.DocumentArtifactRefView {
	views := make([]tui.DocumentArtifactRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.DocumentArtifactRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path})
	}
	return views
}

func documentSourceRefViews(refs []capability.SourceRef) []tui.DocumentSourceRefView {
	views := make([]tui.DocumentSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.DocumentSourceRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func documentBoundaryRequestViews(requests []capability.BoundaryRequest) []tui.DocumentBoundaryRequestView {
	views := make([]tui.DocumentBoundaryRequestView, 0, len(requests))
	for _, request := range requests {
		views = append(views, tui.DocumentBoundaryRequestView{Kind: string(request.Kind), Operation: request.Operation, Target: request.Target, Reason: request.Reason})
	}
	return views
}

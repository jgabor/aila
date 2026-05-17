package capability

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jgabor/aila/internal/workflow"
)

const (
	DocumentMetadataTargetPath       = "document_target_path"
	DocumentMetadataTargetTitle      = "document_target_title"
	DocumentMetadataSourceBehavior   = "document_source_behavior"
	DocumentMetadataPlanID           = "document_plan_id"
	DocumentMetadataPlanSummary      = "document_plan_summary"
	DocumentMetadataPlanSteps        = "document_plan_steps"
	DocumentMetadataOutputSummary    = "document_output_summary"
	DocumentMetadataDiffLines        = "document_diff_lines"
	DocumentMetadataCaveats          = "document_caveats"
	DocumentMetadataNextAction       = "document_next_action"
	DocumentMetadataDurable          = "document_durable"
	DocumentMetadataToolName         = "document_tool_name"
	DocumentMetadataToolStatus       = "document_tool_status"
	DocumentMetadataExpectedEffect   = "document_expected_effect"
	DocumentMetadataDecisionSource   = "document_decision_source"
	DocumentMetadataDecisionAutonomy = "document_decision_autonomy"
	DocumentMetadataDecisionAllowed  = "document_decision_allowed"
	DocumentMetadataApprovalRequired = "document_approval_required"
	DocumentMetadataBytesWritten     = "document_bytes_written"
	DocumentMetadataErrorKind        = "document_error_kind"
	DocumentMetadataErrorMessage     = "document_error_message"
)

const defaultDocumentationArtifactPath = ".aila/artifacts/documentation.md"

// DocumentCapability adapts app-supplied documentation evidence into BUILD-owned alignment output.
type DocumentCapability struct{}

// DocumentOutput is the typed documentation alignment data carried by a document capability exit.
type DocumentOutput struct {
	Target               DocumentTarget
	Plan                 DocumentPlan
	OutputSummary        string
	ChangedDocs          []DocumentChange
	DiffLines            []string
	Mutation             DocumentMutation
	Caveats              []string
	NextAction           string
	DocumentArtifactPath string
	DocumentArtifact     string
	SourceRefs           []SourceRef
}

// DocumentTarget records the bounded documentation target and behavior source.
type DocumentTarget struct {
	Path           string
	Title          string
	SourceBehavior string
}

// DocumentPlan records the bounded documentation alignment plan.
type DocumentPlan struct {
	ID      string
	Summary string
	Steps   []string
}

// DocumentChange records one documentation target touched through mutation safety.
type DocumentChange struct {
	Path    string
	Status  string
	Summary string
}

// DocumentMutation records tool, permission, and history evidence for a doc write.
type DocumentMutation struct {
	ToolName         string
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

// Name returns the fixed capability identity.
func (DocumentCapability) Name() Name {
	return NameDocument
}

// OwningPhase returns BUILD because documentation alignment is build-owned project work.
func (DocumentCapability) OwningPhase() workflow.Phase {
	return workflow.PhaseBuild
}

// Run emits one document payload. File writes must already be handled by app/runtime effects.
func (DocumentCapability) Run(ctx context.Context, request Request) (ExitPayload, error) {
	if err := ctx.Err(); err != nil {
		return ExitPayload{}, fmt.Errorf("run document capability: %w", err)
	}
	request = normalizeDocumentRequest(request)
	invocation := NewInvocation(request)

	if !hasDocumentEvidence(request) {
		payload := ExitPayload{
			Capability:       NameDocument,
			Signal:           ExitWaiting,
			Summary:          "Document needs a target document, source behavior, and alignment plan before writing docs.",
			Concerns:         []string{"documentation alignment evidence unavailable until target, behavior source, and plan are provided"},
			NeededInput:      "Provide a documentation target, source behavior, and alignment plan before documenting.",
			NextAction:       "Provide documentation alignment evidence, then run document again.",
			SourceRefs:       cloneSourceRefs(request.SourceRefs),
			BoundaryRequests: documentBoundaryRequests(request, false),
		}
		return invocation.Emit(payload)
	}

	output := buildDocumentOutput(request)
	signal := ExitComplete
	if documentFlagged(output) {
		signal = ExitFlagged
	}
	successor := workflow.Phase("")
	if signal == ExitComplete && workflow.ValidateProtocolSuccessor(request.Phase, workflow.PhaseAudit) == nil {
		successor = workflow.PhaseAudit
	} else if signal == ExitFlagged && workflow.ValidateProtocolSuccessor(request.Phase, workflow.PhaseBuild) == nil {
		successor = workflow.PhaseBuild
	}

	payload := ExitPayload{
		Capability:           NameDocument,
		Signal:               signal,
		Summary:              documentSummary(output, signal),
		Concerns:             append([]string(nil), output.Caveats...),
		Attempted:            output.Mutation.Status != "",
		NextAction:           output.NextAction,
		RecommendedSuccessor: successor,
		ArtifactRefs:         documentArtifactRefs(output),
		SourceRefs:           cloneSourceRefs(output.SourceRefs),
		BoundaryRequests:     documentBoundaryRequests(request, output.DocumentArtifact != ""),
		Document:             &output,
	}
	return invocation.Emit(payload)
}

func normalizeDocumentRequest(request Request) Request {
	request.Capability = NameDocument
	if request.Phase == "" || request.Phase == workflow.PhaseIdle {
		request.Phase = workflow.PhaseBuild
	}
	request.Metadata = cloneMap(request.Metadata)
	return request
}

func hasDocumentEvidence(request Request) bool {
	return documentMetadata(request, DocumentMetadataTargetPath, "") != "" &&
		documentSourceBehavior(request) != "" &&
		documentMetadata(request, DocumentMetadataPlanSummary, "") != ""
}

func buildDocumentOutput(request Request) DocumentOutput {
	targetPath := documentMetadata(request, DocumentMetadataTargetPath, "docs/aila-documentation-output.md")
	sourceBehavior := documentSourceBehavior(request)
	mutation := documentMutation(request, targetPath)
	plan := DocumentPlan{
		ID:      documentMetadata(request, DocumentMetadataPlanID, "documentation-alignment"),
		Summary: documentMetadata(request, DocumentMetadataPlanSummary, "Align documentation with app-supplied project behavior."),
		Steps:   documentPlanSteps(request),
	}
	outputSummary := documentMetadata(request, DocumentMetadataOutputSummary, fmt.Sprintf("Aligned %s with %s.", targetPath, sourceBehavior))
	caveats := documentListMetadata(request, DocumentMetadataCaveats)
	if len(caveats) == 0 {
		caveats = []string{"deterministic app-supplied documentation evidence only"}
	}
	if mutation.ErrorMessage != "" {
		caveats = append(caveats, mutation.ErrorMessage)
	}
	output := DocumentOutput{
		Target: DocumentTarget{
			Path:           targetPath,
			Title:          documentMetadata(request, DocumentMetadataTargetTitle, "Aila documentation alignment"),
			SourceBehavior: sourceBehavior,
		},
		Plan:                 plan,
		OutputSummary:        outputSummary,
		ChangedDocs:          []DocumentChange{{Path: targetPath, Status: defaultString(mutation.Status, "planned"), Summary: outputSummary}},
		DiffLines:            documentDiffLines(request, targetPath, sourceBehavior),
		Mutation:             mutation,
		Caveats:              caveats,
		NextAction:           documentMetadata(request, DocumentMetadataNextAction, defaultDocumentNextAction(mutation)),
		DocumentArtifactPath: defaultDocumentationArtifactPath,
		SourceRefs:           documentSourceRefs(request, targetPath, sourceBehavior, outputSummary),
	}
	if documentBoolMetadata(request, DocumentMetadataDurable) && !documentFlagged(output) {
		output.DocumentArtifact = documentArtifactDocument(output)
	}
	return output
}

func documentSourceBehavior(request Request) string {
	if behavior := documentMetadata(request, DocumentMetadataSourceBehavior, ""); behavior != "" {
		return behavior
	}
	return strings.TrimSpace(request.Input)
}

func documentPlanSteps(request Request) []string {
	steps := documentListMetadata(request, DocumentMetadataPlanSteps)
	if len(steps) > 0 {
		return steps
	}
	return []string{"Identify behavior evidence", "Update the bounded documentation target", "Record caveats and next action"}
}

func documentMutation(request Request, targetPath string) DocumentMutation {
	return DocumentMutation{
		ToolName:         documentMetadata(request, DocumentMetadataToolName, "write"),
		Status:           documentMetadata(request, DocumentMetadataToolStatus, "completed"),
		Path:             targetPath,
		ExpectedEffect:   documentMetadata(request, DocumentMetadataExpectedEffect, "write bounded documentation alignment output"),
		DecisionSource:   documentMetadata(request, DocumentMetadataDecisionSource, ""),
		DecisionAutonomy: documentMetadata(request, DocumentMetadataDecisionAutonomy, ""),
		DecisionAllowed:  documentBoolMetadata(request, DocumentMetadataDecisionAllowed),
		ApprovalRequired: documentBoolMetadata(request, DocumentMetadataApprovalRequired),
		BytesWritten:     documentIntMetadata(request, DocumentMetadataBytesWritten),
		ErrorKind:        documentMetadata(request, DocumentMetadataErrorKind, ""),
		ErrorMessage:     documentMetadata(request, DocumentMetadataErrorMessage, ""),
	}
}

func documentDiffLines(request Request, targetPath string, sourceBehavior string) []string {
	lines := documentListMetadata(request, DocumentMetadataDiffLines)
	if len(lines) > 0 {
		return lines
	}
	return []string{
		"+ # Aila Documentation Alignment",
		"+ Target: " + targetPath,
		"+ Source behavior: " + sourceBehavior,
	}
}

func documentFlagged(output DocumentOutput) bool {
	switch strings.ToLower(strings.TrimSpace(output.Mutation.Status)) {
	case "denied", "failed", "blocked":
		return true
	}
	return output.Mutation.ErrorKind != "" || output.Mutation.ErrorMessage != ""
}

func documentSummary(output DocumentOutput, signal ExitSignal) string {
	if signal == ExitFlagged {
		return fmt.Sprintf("Document held after %s for %s.", defaultString(output.Mutation.Status, "mutation"), output.Target.Path)
	}
	return fmt.Sprintf("Document aligned %s through normal mutation safety.", output.Target.Path)
}

func defaultDocumentNextAction(mutation DocumentMutation) string {
	if mutation.Status == "denied" || mutation.Status == "failed" || mutation.ErrorMessage != "" {
		return "Review the documentation write result before continuing."
	}
	return "Audit the documentation alignment before continuing."
}

func documentArtifactRefs(output DocumentOutput) []ArtifactRef {
	refs := []ArtifactRef{{ID: "document-target", Kind: "workspace_document", Path: output.Target.Path}}
	if strings.TrimSpace(output.DocumentArtifact) != "" {
		refs = append(refs, ArtifactRef{ID: "documentation-artifact", Kind: "documentation", Path: output.DocumentArtifactPath})
	}
	return refs
}

func documentBoundaryRequests(request Request, durable bool) []BoundaryRequest {
	target := documentMetadata(request, DocumentMetadataTargetPath, "planned documentation target")
	requests := []BoundaryRequest{
		request.RequestStateAccess("documentation.current", "document uses app-supplied documentation alignment evidence"),
		request.RequestToolExecution("write", target, "document writes docs through the runtime mutation tool effect"),
		request.RequestPermissionCheck("write", target, "document requires the permission gate before workspace documentation mutation"),
		request.RequestStateWrite("history", "document records mutation and runtime evidence through app-owned history state"),
		request.RequestArtifactAccess("documentation", "state store resolves the documentation artifact"),
	}
	if durable {
		requests = append(requests, request.RequestStateWrite("documentation", "state store records durable documentation alignment output"))
	}
	return requests
}

func documentSourceRefs(request Request, targetPath string, sourceBehavior string, outputSummary string) []SourceRef {
	refs := cloneSourceRefs(request.SourceRefs)
	ensureRef := func(id, kind, excerpt string) {
		if strings.TrimSpace(excerpt) == "" || hasSourceRef(refs, id) {
			return
		}
		refs = append(refs, SourceRef{ID: id, Kind: kind, Path: targetPath, Excerpt: excerpt})
	}
	ensureRef("document-target", "doc_target", targetPath)
	ensureRef("document-source-behavior", "behavior", sourceBehavior)
	ensureRef("document-output", "doc_output", outputSummary)
	return refs
}

func documentArtifactDocument(output DocumentOutput) string {
	var builder strings.Builder
	builder.WriteString("# Documentation Alignment\n\n")
	builder.WriteString("Target: ")
	builder.WriteString(output.Target.Path)
	builder.WriteString("\n")
	builder.WriteString("Plan: ")
	builder.WriteString(output.Plan.Summary)
	builder.WriteString("\n")
	builder.WriteString("Output: ")
	builder.WriteString(output.OutputSummary)
	builder.WriteString("\n\nChanged docs:\n")
	for _, change := range output.ChangedDocs {
		builder.WriteString("- ")
		builder.WriteString(change.Path)
		builder.WriteString(" [")
		builder.WriteString(change.Status)
		builder.WriteString("]: ")
		builder.WriteString(change.Summary)
		builder.WriteString("\n")
	}
	if len(output.Caveats) > 0 {
		builder.WriteString("\nCaveats:\n")
		for _, caveat := range output.Caveats {
			builder.WriteString("- ")
			builder.WriteString(caveat)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func documentMetadata(request Request, key, fallback string) string {
	if request.Metadata == nil {
		return fallback
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return fallback
	}
	return value
}

func documentListMetadata(request Request, key string) []string {
	if request.Metadata == nil {
		return nil
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return nil
	}
	parts := strings.Split(value, "|")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func documentBoolMetadata(request Request, key string) bool {
	value := strings.ToLower(documentMetadata(request, key, ""))
	return value == "true" || value == "yes" || value == "1" || value == "allowed"
}

func documentIntMetadata(request Request, key string) int {
	value := documentMetadata(request, key, "")
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

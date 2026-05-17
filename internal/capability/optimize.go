package capability

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jgabor/aila/internal/workflow"
)

const (
	OptimizeMetadataObjectiveID       = "optimize_objective_id"
	OptimizeMetadataObjective         = "optimize_objective"
	OptimizeMetadataExperimentID      = "optimize_experiment_id"
	OptimizeMetadataExperimentStatus  = "optimize_experiment_status"
	OptimizeMetadataExperimentSummary = "optimize_experiment_summary"
	OptimizeMetadataHarnessID         = "optimize_harness_id"
	OptimizeMetadataHarnessName       = "optimize_harness_name"
	OptimizeMetadataHarnessCommand    = "optimize_harness_command"
	OptimizeMetadataHarnessLocked     = "optimize_harness_locked"
	OptimizeMetadataMetricName        = "optimize_metric_name"
	OptimizeMetadataMetricBaseline    = "optimize_metric_baseline"
	OptimizeMetadataMetricResult      = "optimize_metric_result"
	OptimizeMetadataMetricUnit        = "optimize_metric_unit"
	OptimizeMetadataMetricDirection   = "optimize_metric_direction"
	OptimizeMetadataMetricImprovement = "optimize_metric_improvement"
	OptimizeMetadataEvidence          = "optimize_evidence"
	OptimizeMetadataCaveats           = "optimize_caveats"
	OptimizeMetadataNextAction        = "optimize_next_action"
	OptimizeMetadataDurable           = "optimize_durable"
)

const (
	defaultObjectiveArtifactPath  = ".aila/artifacts/objective.md"
	defaultExperimentArtifactPath = ".aila/artifacts/experiments.md"
)

// OptimizeCapability adapts app-supplied metric evidence into BUILD-owned optimization output.
type OptimizeCapability struct{}

// OptimizeOutput is the typed metric-driven data carried by an optimize capability exit.
type OptimizeOutput struct {
	Objective              OptimizeObjective
	Experiment             OptimizeExperiment
	Harness                OptimizeHarness
	Metric                 OptimizeMetric
	Evidence               []OptimizeEvidence
	Caveats                []string
	NextAction             string
	ObjectiveArtifactPath  string
	ExperimentArtifactPath string
	ObjectiveDocument      string
	ExperimentDocument     string
	SourceRefs             []SourceRef
}

// OptimizeObjective records the measured optimization target.
type OptimizeObjective struct {
	ID   string
	Text string
}

// OptimizeExperiment records one app-supplied experiment status.
type OptimizeExperiment struct {
	ID      string
	Status  string
	Summary string
}

// OptimizeHarness records the locked measurement harness.
type OptimizeHarness struct {
	ID      string
	Name    string
	Command string
	Locked  bool
}

// OptimizeMetric records the baseline and result for a measured target.
type OptimizeMetric struct {
	Name        string
	Baseline    string
	Result      string
	Unit        string
	Direction   string
	Improvement string
}

// OptimizeEvidence records one source-backed optimization observation.
type OptimizeEvidence struct {
	ID          string
	Summary     string
	SourceRefID string
}

// Name returns the fixed capability identity.
func (OptimizeCapability) Name() Name {
	return NameOptimize
}

// OwningPhase returns BUILD because optimization is metric-driven build work.
func (OptimizeCapability) OwningPhase() workflow.Phase {
	return workflow.PhaseBuild
}

// Run emits one optimize payload without running benchmarks, tools, or phase transitions itself.
func (OptimizeCapability) Run(ctx context.Context, request Request) (ExitPayload, error) {
	if err := ctx.Err(); err != nil {
		return ExitPayload{}, fmt.Errorf("run optimize capability: %w", err)
	}
	request = normalizeOptimizeRequest(request)
	invocation := NewInvocation(request)

	if !hasOptimizeEvidence(request) {
		payload := ExitPayload{
			Capability:       NameOptimize,
			Signal:           ExitWaiting,
			Summary:          "Optimize needs an objective, locked harness, metric baseline, and measured result.",
			Concerns:         []string{"optimization evidence unavailable until objective, harness, and metric result are provided"},
			NeededInput:      "Provide objective, locked harness, metric baseline, and measured result before optimizing.",
			NextAction:       "Provide locked metric evidence, then run optimize again.",
			SourceRefs:       cloneSourceRefs(request.SourceRefs),
			BoundaryRequests: optimizeBoundaryRequests(request, false),
		}
		return invocation.Emit(payload)
	}

	output := buildOptimizeOutput(request)
	signal := ExitComplete
	if optimizeFlagged(output) {
		signal = ExitFlagged
	}
	successor := workflow.Phase("")
	if signal == ExitComplete && workflow.ValidateProtocolSuccessor(request.Phase, workflow.PhaseAudit) == nil {
		successor = workflow.PhaseAudit
	} else if signal == ExitFlagged && workflow.ValidateProtocolSuccessor(request.Phase, workflow.PhaseBuild) == nil {
		successor = workflow.PhaseBuild
	}

	payload := ExitPayload{
		Capability:           NameOptimize,
		Signal:               signal,
		Summary:              optimizeSummary(output, signal),
		Concerns:             append([]string(nil), output.Caveats...),
		Attempted:            true,
		NextAction:           output.NextAction,
		RecommendedSuccessor: successor,
		ArtifactRefs:         optimizeArtifactRefs(output),
		SourceRefs:           cloneSourceRefs(output.SourceRefs),
		BoundaryRequests:     optimizeBoundaryRequests(request, output.ObjectiveDocument != "" || output.ExperimentDocument != ""),
		Optimize:             &output,
	}
	return invocation.Emit(payload)
}

func normalizeOptimizeRequest(request Request) Request {
	request.Capability = NameOptimize
	if request.Phase == "" || request.Phase == workflow.PhaseIdle {
		request.Phase = workflow.PhaseBuild
	}
	request.Metadata = cloneMap(request.Metadata)
	return request
}

func hasOptimizeEvidence(request Request) bool {
	return optimizeObjectiveText(request) != "" &&
		optimizeMetadata(request, OptimizeMetadataHarnessName, "") != "" &&
		optimizeMetadata(request, OptimizeMetadataHarnessLocked, "") != "" &&
		optimizeMetadata(request, OptimizeMetadataMetricName, "") != "" &&
		optimizeMetadata(request, OptimizeMetadataMetricBaseline, "") != "" &&
		optimizeMetadata(request, OptimizeMetadataMetricResult, "") != ""
}

func buildOptimizeOutput(request Request) OptimizeOutput {
	metric := OptimizeMetric{
		Name:      optimizeMetadata(request, OptimizeMetadataMetricName, ""),
		Baseline:  optimizeMetadata(request, OptimizeMetadataMetricBaseline, ""),
		Result:    optimizeMetadata(request, OptimizeMetadataMetricResult, ""),
		Unit:      optimizeMetadata(request, OptimizeMetadataMetricUnit, ""),
		Direction: optimizeMetricDirection(request),
	}
	metric.Improvement = optimizeMetadata(request, OptimizeMetadataMetricImprovement, optimizeImprovement(metric))

	harness := OptimizeHarness{
		ID:      optimizeMetadata(request, OptimizeMetadataHarnessID, "metric-harness"),
		Name:    optimizeMetadata(request, OptimizeMetadataHarnessName, "locked metric harness"),
		Command: optimizeMetadata(request, OptimizeMetadataHarnessCommand, ""),
		Locked:  optimizeBoolMetadata(request, OptimizeMetadataHarnessLocked),
	}
	evidence := optimizeEvidence(request, optimizeSourceRefs(request))
	status := optimizeMetadata(request, OptimizeMetadataExperimentStatus, defaultOptimizeExperimentStatus(metric))
	caveats := optimizeListMetadata(request, OptimizeMetadataCaveats)
	if len(caveats) == 0 {
		caveats = []string{"optimization used app-supplied metric evidence only"}
	}
	if !harness.Locked {
		caveats = append(caveats, "metric harness is not locked")
	}
	output := OptimizeOutput{
		Objective: OptimizeObjective{
			ID:   optimizeMetadata(request, OptimizeMetadataObjectiveID, "objective"),
			Text: optimizeObjectiveText(request),
		},
		Experiment: OptimizeExperiment{
			ID:      optimizeMetadata(request, OptimizeMetadataExperimentID, "experiment-1"),
			Status:  status,
			Summary: optimizeMetadata(request, OptimizeMetadataExperimentSummary, optimizeExperimentSummary(metric, status)),
		},
		Harness:                harness,
		Metric:                 metric,
		Evidence:               evidence,
		Caveats:                caveats,
		NextAction:             optimizeMetadata(request, OptimizeMetadataNextAction, defaultOptimizeNextAction(status, harness.Locked)),
		ObjectiveArtifactPath:  defaultObjectiveArtifactPath,
		ExperimentArtifactPath: defaultExperimentArtifactPath,
		SourceRefs:             optimizeSourceRefs(request),
	}
	if optimizeDurable(request) {
		output.ObjectiveDocument = optimizeObjectiveDocument(output)
		output.ExperimentDocument = optimizeExperimentDocument(output)
	}
	return output
}

func optimizeObjectiveText(request Request) string {
	if objective := optimizeMetadata(request, OptimizeMetadataObjective, ""); objective != "" {
		return objective
	}
	return strings.TrimSpace(request.Input)
}

func optimizeMetricDirection(request Request) string {
	direction := strings.ToLower(optimizeMetadata(request, OptimizeMetadataMetricDirection, "lower"))
	switch direction {
	case "higher", "increase", "up":
		return "higher"
	default:
		return "lower"
	}
}

func defaultOptimizeExperimentStatus(metric OptimizeMetric) string {
	if optimizeMetricImproved(metric) {
		return "improved"
	}
	return "measured"
}

func optimizeExperimentSummary(metric OptimizeMetric, status string) string {
	if metric.Name == "" {
		return "Optimization experiment recorded supplied metric evidence."
	}
	unit := metric.Unit
	return fmt.Sprintf("%s %s moved from %s%s to %s%s.", status, metric.Name, metric.Baseline, unit, metric.Result, unit)
}

func defaultOptimizeNextAction(status string, locked bool) string {
	if !locked {
		return "Lock the harness before continuing optimization."
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "regressed", "blocked":
		return "Review the regression and route back through build before auditing."
	default:
		return "Audit the measured optimization result before continuing."
	}
}

func optimizeFlagged(output OptimizeOutput) bool {
	if !output.Harness.Locked {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(output.Experiment.Status)) {
	case "failed", "regressed", "blocked":
		return true
	default:
		return false
	}
}

func optimizeSummary(output OptimizeOutput, signal ExitSignal) string {
	if signal == ExitFlagged {
		return fmt.Sprintf("Optimize flagged %s for %s with metric %s.", output.Experiment.Status, output.Objective.ID, output.Metric.Name)
	}
	return fmt.Sprintf("Optimize measured %s for %s: %s -> %s%s.", output.Metric.Name, output.Objective.ID, output.Metric.Baseline, output.Metric.Result, output.Metric.Unit)
}

func optimizeArtifactRefs(output OptimizeOutput) []ArtifactRef {
	if strings.TrimSpace(output.ObjectiveDocument) == "" && strings.TrimSpace(output.ExperimentDocument) == "" {
		return nil
	}
	return []ArtifactRef{
		{ID: "objective-artifact", Kind: "objective", Path: output.ObjectiveArtifactPath},
		{ID: "experiments-artifact", Kind: "experiments", Path: output.ExperimentArtifactPath},
	}
}

func optimizeBoundaryRequests(request Request, durable bool) []BoundaryRequest {
	harnessTarget := optimizeMetadata(request, OptimizeMetadataHarnessCommand, optimizeMetadata(request, OptimizeMetadataHarnessName, "locked metric harness"))
	requests := []BoundaryRequest{
		request.RequestStateAccess("objective.current", "optimize uses app-supplied objective state"),
		request.RequestToolExecution("bash", harnessTarget, "optimize execution stays on the normal tool effect path"),
		request.RequestPermissionCheck("bash", harnessTarget, "optimize metric harness execution is permission-gated"),
		request.RequestArtifactAccess("objective", "state store resolves the optimization objective artifact"),
		request.RequestArtifactAccess("experiments", "state store resolves the optimization experiment artifact"),
	}
	if durable {
		requests = append(requests,
			request.RequestStateWrite("objective", "state store records durable optimization objective output"),
			request.RequestStateWrite("experiments", "state store records durable optimization experiment output"),
		)
	}
	return requests
}

func optimizeEvidence(request Request, sourceRefs []SourceRef) []OptimizeEvidence {
	items := optimizeListMetadata(request, OptimizeMetadataEvidence)
	evidence := make([]OptimizeEvidence, 0, len(items))
	for index, item := range items {
		refID := fmt.Sprintf("optimize-source-%d", index+1)
		if len(sourceRefs) > 0 {
			refID = sourceRefs[index%len(sourceRefs)].ID
		}
		evidence = append(evidence, OptimizeEvidence{ID: fmt.Sprintf("evidence-%d", index+1), Summary: item, SourceRefID: refID})
	}
	return evidence
}

func optimizeSourceRefs(request Request) []SourceRef {
	refs := cloneSourceRefs(request.SourceRefs)
	ensureRef := func(id, kind, excerpt string) {
		if strings.TrimSpace(excerpt) == "" || hasSourceRef(refs, id) {
			return
		}
		refs = append(refs, SourceRef{ID: id, Kind: kind, Excerpt: excerpt})
	}
	ensureRef("optimize-objective", "objective", optimizeObjectiveText(request))
	ensureRef("optimize-harness", "metric_harness", optimizeMetadata(request, OptimizeMetadataHarnessName, ""))
	ensureRef("optimize-metric", "metric_result", optimizeExperimentSummary(OptimizeMetric{
		Name:     optimizeMetadata(request, OptimizeMetadataMetricName, ""),
		Baseline: optimizeMetadata(request, OptimizeMetadataMetricBaseline, ""),
		Result:   optimizeMetadata(request, OptimizeMetadataMetricResult, ""),
		Unit:     optimizeMetadata(request, OptimizeMetadataMetricUnit, ""),
	}, optimizeMetadata(request, OptimizeMetadataExperimentStatus, "measured")))
	return refs
}

func optimizeObjectiveDocument(output OptimizeOutput) string {
	var builder strings.Builder
	builder.WriteString("# Optimization Objective\n\n")
	builder.WriteString("ID: ")
	builder.WriteString(output.Objective.ID)
	builder.WriteString("\n\n")
	builder.WriteString(output.Objective.Text)
	builder.WriteString("\n")
	return builder.String()
}

func optimizeExperimentDocument(output OptimizeOutput) string {
	var builder strings.Builder
	builder.WriteString("# Optimization Experiment\n\n")
	builder.WriteString("Experiment: ")
	builder.WriteString(output.Experiment.ID)
	builder.WriteString("\n")
	builder.WriteString("Status: ")
	builder.WriteString(output.Experiment.Status)
	builder.WriteString("\n")
	builder.WriteString("Harness: ")
	builder.WriteString(output.Harness.Name)
	builder.WriteString(" locked=")
	builder.WriteString(strconv.FormatBool(output.Harness.Locked))
	builder.WriteString("\n")
	builder.WriteString("Metric: ")
	builder.WriteString(output.Metric.Name)
	builder.WriteString(" ")
	builder.WriteString(output.Metric.Baseline)
	builder.WriteString(output.Metric.Unit)
	builder.WriteString(" -> ")
	builder.WriteString(output.Metric.Result)
	builder.WriteString(output.Metric.Unit)
	if output.Metric.Improvement != "" {
		builder.WriteString(" (")
		builder.WriteString(output.Metric.Improvement)
		builder.WriteString(")")
	}
	builder.WriteString("\n\n")
	builder.WriteString(output.Experiment.Summary)
	builder.WriteString("\n")
	return builder.String()
}

func optimizeImprovement(metric OptimizeMetric) string {
	baseline, err := strconv.ParseFloat(strings.TrimSpace(metric.Baseline), 64)
	if err != nil || baseline == 0 {
		return ""
	}
	result, err := strconv.ParseFloat(strings.TrimSpace(metric.Result), 64)
	if err != nil {
		return ""
	}
	var ratio float64
	if metric.Direction == "higher" {
		ratio = (result - baseline) / baseline * 100
		return fmt.Sprintf("%.1f%% higher", ratio)
	}
	ratio = (baseline - result) / baseline * 100
	return fmt.Sprintf("%.1f%% lower", ratio)
}

func optimizeMetricImproved(metric OptimizeMetric) bool {
	baseline, err := strconv.ParseFloat(strings.TrimSpace(metric.Baseline), 64)
	if err != nil {
		return false
	}
	result, err := strconv.ParseFloat(strings.TrimSpace(metric.Result), 64)
	if err != nil {
		return false
	}
	if metric.Direction == "higher" {
		return result >= baseline
	}
	return result <= baseline
}

func optimizeDurable(request Request) bool {
	return optimizeBoolMetadata(request, OptimizeMetadataDurable)
}

func optimizeMetadata(request Request, key, fallback string) string {
	if request.Metadata == nil {
		return fallback
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return fallback
	}
	return value
}

func optimizeListMetadata(request Request, key string) []string {
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

func optimizeBoolMetadata(request Request, key string) bool {
	value := strings.ToLower(optimizeMetadata(request, key, ""))
	return value == "true" || value == "yes" || value == "1" || value == "locked"
}

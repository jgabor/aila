package app

import (
	"strings"

	"github.com/jgabor/aila/internal/agent"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/tui"
)

const modelSwitchChoiceLimit = 16

func (controller *sessionController) openModelSwitchView(recommendation policy.CommandRecommendation) {
	target := recommendation.Target
	if target == policy.CommandTargetNone {
		target = policy.CommandTargetPrimaryModel
	}
	if recommendation.Selection != "" {
		controller.applyModelSelection(target, recommendation.Selection)
	}
	current := controller.currentModelLabel(target)
	items := controller.modelSwitchItems(target)
	selected := selectedModelSwitchIndex(items, current)
	detail := "session-scoped model selection; config file unchanged"
	focus := recommendation.Selection == ""
	if recommendation.Selection != "" {
		detail = "applied " + current + " to current session; config file unchanged"
	}
	controller.view = tui.ApplyModelSwitchView(controller.view, &tui.ModelSwitchView{
		Target:         string(target),
		Source:         "app.model",
		Status:         "ready",
		CurrentPrimary: controller.view.PrimaryModel,
		CurrentUtility: controller.view.UtilityModel,
		Detail:         detail,
		Items:          items,
		Selected:       selected,
		Focus:          focus,
	})
}

func (controller *sessionController) applyModelSelection(target policy.CommandTarget, selection string) {
	selection = strings.TrimSpace(selection)
	if selection == "" {
		return
	}
	switch target {
	case policy.CommandTargetUtilityModel:
		controller.view.UtilityModel = selection
	default:
		controller.view.PrimaryModel = selection
		controller.updateAgentModel(selection)
	}
}

func (controller *sessionController) updateAgentModel(label string) {
	if controller.runner == nil || controller.runner.agent == nil {
		return
	}
	ref, err := ParseModelRef(label)
	if err != nil {
		return
	}
	controller.runner.agent.provider = ref.Provider
	controller.runner.agent.model = ref.Model
}

func (controller *sessionController) currentModelLabel(target policy.CommandTarget) string {
	if target == policy.CommandTargetUtilityModel {
		return controller.view.UtilityModel
	}
	return controller.view.PrimaryModel
}

func (controller *sessionController) modelSwitchItems(target policy.CommandTarget) []tui.ModelSwitchItemView {
	current := controller.currentModelLabel(target)
	items := []tui.ModelSwitchItemView{modelSwitchItemFromLabel(current, true, "current session "+modelTargetName(target)+" model")}
	diagnostics := agent.ListFakeModelDiagnostics(agent.ModelDiagnosticFilter{})
	if target == policy.CommandTargetUtilityModel {
		diagnostics = utilityFirstDiagnostics(diagnostics)
	}
	seen := map[string]bool{current: true}
	for _, diagnostic := range diagnostics {
		item := modelSwitchItemFromDiagnostic(diagnostic)
		if seen[item.Label] {
			continue
		}
		seen[item.Label] = true
		items = append(items, item)
		if len(items) >= modelSwitchChoiceLimit {
			break
		}
	}
	return items
}

func utilityFirstDiagnostics(diagnostics []agent.ModelDiagnostic) []agent.ModelDiagnostic {
	ordered := append([]agent.ModelDiagnostic(nil), diagnostics...)
	stablePartition(ordered, func(diagnostic agent.ModelDiagnostic) bool { return diagnostic.Class == "utility" })
	return ordered
}

func stablePartition(values []agent.ModelDiagnostic, first func(agent.ModelDiagnostic) bool) {
	var left []agent.ModelDiagnostic
	var right []agent.ModelDiagnostic
	for _, value := range values {
		if first(value) {
			left = append(left, value)
			continue
		}
		right = append(right, value)
	}
	copy(values, append(left, right...))
}

func modelSwitchItemFromLabel(label string, current bool, detail string) tui.ModelSwitchItemView {
	item := tui.ModelSwitchItemView{
		Label:            label,
		SourceName:       "current",
		Model:            label,
		Family:           "session",
		Class:            "current",
		Status:           "current",
		CredentialSource: "not inspected",
		Detail:           detail,
		Current:          current,
	}
	ref, err := ParseModelRef(label)
	if err != nil {
		return item
	}
	item.SourceName = ref.Provider
	item.Model = ref.Model
	item.Reasoning = ref.Reasoning
	readiness, err := agent.ClassifyFakeReadiness(agent.ReadinessRequest{Provider: ref.Provider, Model: ref.Model, Reasoning: ref.Reasoning})
	if err != nil {
		item.Family = "unknown"
		item.Class = "unknown"
		item.Status = "current"
		return item
	}
	item.Family = string(readiness.Family)
	item.Class = modelClass(readiness)
	item.CredentialSource = credentialSource(readiness)
	return item
}

func modelSwitchItemFromDiagnostic(diagnostic agent.ModelDiagnostic) tui.ModelSwitchItemView {
	label := diagnostic.Provider + "/" + diagnostic.Model
	credential := "not inspected"
	readiness, err := agent.ClassifyFakeReadiness(agent.ReadinessRequest{Provider: diagnostic.Provider, Model: diagnostic.Model})
	if err == nil {
		credential = credentialSource(readiness)
	}
	detail := "deterministic readiness row"
	if diagnostic.Error != "" && diagnostic.Error != "-" {
		detail = diagnostic.Error
	}
	return tui.ModelSwitchItemView{
		Label:            label,
		SourceName:       diagnostic.Provider,
		Model:            diagnostic.Model,
		Family:           string(diagnostic.Family),
		Class:            diagnostic.Class,
		Status:           string(diagnostic.Status),
		CredentialSource: credential,
		Detail:           detail,
	}
}

func modelClass(readiness agent.ProviderReadiness) string {
	for _, entry := range readiness.Metadata {
		if entry.Name == "model_class" {
			return entry.Value
		}
	}
	return "unknown"
}

func credentialSource(readiness agent.ProviderReadiness) string {
	if len(readiness.CredentialSourceNames) == 0 {
		return "not inspected"
	}
	return strings.Join(readiness.CredentialSourceNames, ",")
}

func selectedModelSwitchIndex(items []tui.ModelSwitchItemView, current string) int {
	for index, item := range items {
		if item.Label == current {
			return index
		}
	}
	return 0
}

func modelTargetName(target policy.CommandTarget) string {
	if target == policy.CommandTargetUtilityModel {
		return "utility"
	}
	return "primary"
}

func (controller *sessionController) openAutonomySwitchView(recommendation policy.CommandRecommendation) {
	if recommendation.Selection != "" {
		controller.applyAutonomySelection(recommendation.Selection)
	}
	items := autonomySwitchItems(controller.view.Autonomy)
	detail := "session-scoped autonomy selection; config file unchanged"
	focus := recommendation.Selection == ""
	if recommendation.Selection != "" {
		detail = "applied " + controller.view.Autonomy + " autonomy to current session; config file unchanged"
	}
	controller.view = tui.ApplyAutonomySwitchView(controller.view, &tui.AutonomySwitchView{
		Source:   "app.autonomy",
		Status:   "ready",
		Current:  controller.view.Autonomy,
		Detail:   detail,
		Items:    items,
		Selected: selectedAutonomySwitchIndex(items, controller.view.Autonomy),
		Focus:    focus,
	})
}

func (controller *sessionController) applyAutonomySelection(selection string) {
	selection = strings.TrimSpace(selection)
	if !validAutonomySelection(selection) {
		return
	}
	controller.view.Autonomy = selection
	controller.autonomyLevel = selection
	if controller.runner != nil {
		controller.runner.dispatch = readDispatchContext(controller.ctx, controller.workspacePath, selection)
	}
}

func autonomySwitchItems(current string) []tui.AutonomySwitchItemView {
	choices := []struct {
		level  string
		detail string
	}{
		{level: "off", detail: "approval required before read or write operations"},
		{level: "read", detail: "read-only operations may run automatically"},
		{level: "write", detail: "workspace write operations may run automatically"},
		{level: "yolo", detail: "highest autonomy for classified operations"},
	}
	items := make([]tui.AutonomySwitchItemView, 0, len(choices))
	for _, choice := range choices {
		items = append(items, tui.AutonomySwitchItemView{Level: choice.level, Status: "available", Detail: choice.detail, Current: choice.level == current})
	}
	return items
}

func selectedAutonomySwitchIndex(items []tui.AutonomySwitchItemView, current string) int {
	for index, item := range items {
		if item.Level == current {
			return index
		}
	}
	return 0
}

func validAutonomySelection(selection string) bool {
	switch selection {
	case "off", "read", "write", "yolo":
		return true
	default:
		return false
	}
}

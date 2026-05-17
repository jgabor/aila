package app

import (
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
)

func runtimeActive(model runtime.Model) bool {
	if model.Status == runtime.StatusActive || model.Status == runtime.StatusApprovalPending || model.Status == runtime.StatusCanceling {
		return true
	}
	for _, run := range model.Subagents {
		if run.Status.Active() {
			return true
		}
	}
	return false
}

func subagentViews(model runtime.Model) []tui.SubagentView {
	if len(model.Subagents) == 0 {
		return nil
	}
	views := make([]tui.SubagentView, 0, len(model.Subagents))
	for _, run := range model.Subagents {
		views = append(views, tui.SubagentView{
			ID:                run.ID,
			ParentRunID:       run.ParentRunID,
			Purpose:           run.Purpose,
			Status:            string(run.Status),
			Summary:           run.Summary,
			EvidenceLinks:     subagentEvidenceLinkViews(run.EvidenceLinks),
			DisplayOnly:       true,
			TransitionClaimed: false,
		})
	}
	return views
}

func subagentEvidenceLinkViews(links []runtime.SubagentEvidenceLink) []tui.SubagentEvidenceLinkView {
	if len(links) == 0 {
		return nil
	}
	views := make([]tui.SubagentEvidenceLinkView, 0, len(links))
	for _, link := range links {
		views = append(views, tui.SubagentEvidenceLinkView{ID: link.ID, Kind: link.Kind, Path: link.Path, Command: link.Command, Excerpt: link.Excerpt})
	}
	return views
}

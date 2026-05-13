package app

import (
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/agent"
)

const maxModelDiagnosticRows = 64

func ModelsCommandOutput(version string, args []string) (string, error) {
	filter, err := parseModelDiagnosticFilter(args)
	if err != nil {
		return "", err
	}

	diagnostics := agent.ListFakeModelDiagnostics(filter)
	if len(diagnostics) > maxModelDiagnosticRows {
		diagnostics = diagnostics[:maxModelDiagnosticRows]
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "aila %s\n", version)
	builder.WriteString("command: models\n")
	builder.WriteString("status: fake diagnostics\n")
	fmt.Fprintf(&builder, "filters: %s\n", displayModelFilters(args))
	builder.WriteString("columns: provider model family class status error\n")
	for _, diagnostic := range diagnostics {
		fmt.Fprintf(&builder, "%s %s %s %s %s %s\n", diagnostic.Provider, diagnostic.Model, diagnostic.Family, diagnostic.Class, diagnostic.Status, diagnostic.Error)
	}
	fmt.Fprintf(&builder, "count: %d\n", len(diagnostics))
	builder.WriteString("source: deterministic-fakes\n")
	return builder.String(), nil
}

func parseModelDiagnosticFilter(args []string) (agent.ModelDiagnosticFilter, error) {
	filter := agent.ModelDiagnosticFilter{}
	for _, arg := range args {
		key, value, hasKey := strings.Cut(arg, "=")
		if hasKey {
			if value == "" {
				return agent.ModelDiagnosticFilter{}, fmt.Errorf("missing value for models filter %q; valid filters: provider=, status=, family=, class=, or search tokens", key)
			}
			switch key {
			case "provider":
				filter.Provider = value
			case "status":
				filter.Status = agent.ModelDiagnosticStatus(value)
			case "family":
				filter.Family = agent.ProviderFamily(value)
			case "class":
				filter.Class = value
			default:
				return agent.ModelDiagnosticFilter{}, fmt.Errorf("unknown models filter %q; valid filters: provider=, status=, family=, class=, or search tokens", key)
			}
			continue
		}
		filter.Search = append(filter.Search, arg)
	}
	return filter, nil
}

func displayModelFilters(args []string) string {
	if len(args) == 0 {
		return "none"
	}
	return strings.Join(args, ",")
}

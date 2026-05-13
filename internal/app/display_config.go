package app

import "github.com/jgabor/aila/internal/tui"

const (
	defaultPrimaryModel = "opencode-go/deepseek-v4-pro:high"
	defaultUtilityModel = "opencode-go/deepseek-v4-flash:max"
	defaultAutonomy     = "yolo"
)

// DisplayConfig is app-owned presentation data for configuration labels.
type DisplayConfig struct {
	PrimaryModel string
	UtilityModel string
	Autonomy     string
}

// DefaultDisplayConfig returns README default labels without reading user config.
func DefaultDisplayConfig() DisplayConfig {
	return DisplayConfigFromConfig(DefaultConfig())
}

// DisplayConfigFromConfig converts loaded user config into TUI display labels.
func DisplayConfigFromConfig(config Config) DisplayConfig {
	return DisplayConfig{
		PrimaryModel: config.LLM.Model.Label,
		UtilityModel: config.LLM.Utility.Model.Label,
		Autonomy:     config.Autonomy.Level,
	}
}

// NewDisplayState applies display labels to an existing TUI view state.
func NewDisplayState(base tui.ViewState, config DisplayConfig) tui.ViewState {
	base.PrimaryModel = config.PrimaryModel
	base.UtilityModel = config.UtilityModel
	base.Autonomy = config.Autonomy
	return base
}

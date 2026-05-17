package policy

import "strings"

// CommandRoute is Aila's fixed command recommendation set.
type CommandRoute string

const (
	CommandRouteNone     CommandRoute = ""
	CommandRouteNew      CommandRoute = "new"
	CommandRouteClear    CommandRoute = "clear"
	CommandRouteContinue CommandRoute = "continue"
	CommandRouteEditor   CommandRoute = "editor"
	CommandRouteModel    CommandRoute = "model"
	CommandRouteAuto     CommandRoute = "auto"
	CommandRouteStatus   CommandRoute = "status"
	CommandRouteVision   CommandRoute = "vision"
	CommandRouteDiscuss  CommandRoute = "discuss"
	CommandRouteResearch CommandRoute = "research"
	CommandRouteProfile  CommandRoute = "profile"
	CommandRoutePlan     CommandRoute = "plan"
	CommandRouteBuild    CommandRoute = "build"
	CommandRouteOptimize CommandRoute = "optimize"
	CommandRouteReview   CommandRoute = "review"
	CommandRouteHelp     CommandRoute = "help"
	CommandRouteHistory  CommandRoute = "history"
	CommandRouteCompact  CommandRoute = "compact"
	CommandRouteDiff     CommandRoute = "diff"
	CommandRouteUndo     CommandRoute = "undo"
	CommandRouteRedo     CommandRoute = "redo"
	CommandRouteQuit     CommandRoute = "quit"
)

// CommandTarget identifies the closed command target selected by a fixed route.
type CommandTarget string

const (
	CommandTargetNone         CommandTarget = ""
	CommandTargetPrimaryModel CommandTarget = "primary_model"
	CommandTargetUtilityModel CommandTarget = "utility_model"
	CommandTargetAutonomy     CommandTarget = "autonomy"
)

// CommandInputKind identifies the closed command input family that produced a route.
type CommandInputKind string

const (
	CommandInputSlash     CommandInputKind = "slash"
	CommandInputShortcut  CommandInputKind = "shortcut"
	CommandInputSelection CommandInputKind = "selection"
)

// CommandRecommendation is a policy-owned recommendation for a fixed command route.
type CommandRecommendation struct {
	Route     CommandRoute
	Kind      CommandInputKind
	Target    CommandTarget
	Selection string
}

// RecommendSlashCommand maps exact slash commands to closed command routes.
func RecommendSlashCommand(input string) (CommandRecommendation, bool) {
	switch strings.TrimSpace(input) {
	case "/new":
		return CommandRecommendation{Route: CommandRouteNew, Kind: CommandInputSlash}, true
	case "/clear":
		return CommandRecommendation{Route: CommandRouteClear, Kind: CommandInputSlash}, true
	case "/continue":
		return CommandRecommendation{Route: CommandRouteContinue, Kind: CommandInputSlash}, true
	case "/editor":
		return CommandRecommendation{Route: CommandRouteEditor, Kind: CommandInputSlash}, true
	case "/model":
		return CommandRecommendation{Route: CommandRouteModel, Kind: CommandInputSlash, Target: CommandTargetPrimaryModel}, true
	case "/model --utility":
		return CommandRecommendation{Route: CommandRouteModel, Kind: CommandInputSlash, Target: CommandTargetUtilityModel}, true
	case "/auto":
		return CommandRecommendation{Route: CommandRouteAuto, Kind: CommandInputSlash, Target: CommandTargetAutonomy}, true
	case "/status":
		return CommandRecommendation{Route: CommandRouteStatus, Kind: CommandInputSlash}, true
	case "/vision":
		return CommandRecommendation{Route: CommandRouteVision, Kind: CommandInputSlash}, true
	case "/discuss":
		return CommandRecommendation{Route: CommandRouteDiscuss, Kind: CommandInputSlash}, true
	case "/research":
		return CommandRecommendation{Route: CommandRouteResearch, Kind: CommandInputSlash}, true
	case "/profile":
		return CommandRecommendation{Route: CommandRouteProfile, Kind: CommandInputSlash}, true
	case "/plan":
		return CommandRecommendation{Route: CommandRoutePlan, Kind: CommandInputSlash}, true
	case "/build":
		return CommandRecommendation{Route: CommandRouteBuild, Kind: CommandInputSlash}, true
	case "/optimize":
		return CommandRecommendation{Route: CommandRouteOptimize, Kind: CommandInputSlash}, true
	case "/review":
		return CommandRecommendation{Route: CommandRouteReview, Kind: CommandInputSlash}, true
	case "/help":
		return CommandRecommendation{Route: CommandRouteHelp, Kind: CommandInputSlash}, true
	case "/history":
		return CommandRecommendation{Route: CommandRouteHistory, Kind: CommandInputSlash}, true
	case "/compact":
		return CommandRecommendation{Route: CommandRouteCompact, Kind: CommandInputSlash}, true
	case "/diff":
		return CommandRecommendation{Route: CommandRouteDiff, Kind: CommandInputSlash}, true
	case "/undo":
		return CommandRecommendation{Route: CommandRouteUndo, Kind: CommandInputSlash}, true
	case "/redo":
		return CommandRecommendation{Route: CommandRouteRedo, Kind: CommandInputSlash}, true
	case "/quit":
		return CommandRecommendation{Route: CommandRouteQuit, Kind: CommandInputSlash}, true
	default:
		return CommandRecommendation{}, false
	}
}

// RecommendShortcut maps exact ctrl+x shortcuts to the same closed routes as slash commands.
func RecommendShortcut(prefix, key string) (CommandRecommendation, bool) {
	if prefix != "ctrl+x" {
		return CommandRecommendation{}, false
	}

	switch key {
	case "n":
		return CommandRecommendation{Route: CommandRouteNew, Kind: CommandInputShortcut}, true
	case "c":
		return CommandRecommendation{Route: CommandRouteContinue, Kind: CommandInputShortcut}, true
	case "e":
		return CommandRecommendation{Route: CommandRouteEditor, Kind: CommandInputShortcut}, true
	case "m":
		return CommandRecommendation{Route: CommandRouteModel, Kind: CommandInputShortcut, Target: CommandTargetPrimaryModel}, true
	case "a":
		return CommandRecommendation{Route: CommandRouteAuto, Kind: CommandInputShortcut, Target: CommandTargetAutonomy}, true
	case "s":
		return CommandRecommendation{Route: CommandRouteStatus, Kind: CommandInputShortcut}, true
	case "i":
		return CommandRecommendation{Route: CommandRouteReview, Kind: CommandInputShortcut}, true
	case "h":
		return CommandRecommendation{Route: CommandRouteHistory, Kind: CommandInputShortcut}, true
	case "k":
		return CommandRecommendation{Route: CommandRouteCompact, Kind: CommandInputShortcut}, true
	case "d":
		return CommandRecommendation{Route: CommandRouteDiff, Kind: CommandInputShortcut}, true
	case "u":
		return CommandRecommendation{Route: CommandRouteUndo, Kind: CommandInputShortcut}, true
	case "r":
		return CommandRecommendation{Route: CommandRouteRedo, Kind: CommandInputShortcut}, true
	case "q":
		return CommandRecommendation{Route: CommandRouteQuit, Kind: CommandInputShortcut}, true
	default:
		return CommandRecommendation{}, false
	}
}

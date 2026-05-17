package policy

import "strings"

// CommandRoute is Aila's fixed command recommendation set.
type CommandRoute string

const (
	CommandRouteNone    CommandRoute = ""
	CommandRouteStatus  CommandRoute = "status"
	CommandRouteReview  CommandRoute = "review"
	CommandRouteHelp    CommandRoute = "help"
	CommandRouteHistory CommandRoute = "history"
	CommandRouteDiff    CommandRoute = "diff"
	CommandRouteUndo    CommandRoute = "undo"
	CommandRouteRedo    CommandRoute = "redo"
	CommandRouteQuit    CommandRoute = "quit"
)

// CommandInputKind identifies the closed command input family that produced a route.
type CommandInputKind string

const (
	CommandInputSlash    CommandInputKind = "slash"
	CommandInputShortcut CommandInputKind = "shortcut"
)

// CommandRecommendation is a policy-owned recommendation for a fixed command route.
type CommandRecommendation struct {
	Route CommandRoute
	Kind  CommandInputKind
}

// RecommendSlashCommand maps exact slash commands to closed command routes.
func RecommendSlashCommand(input string) (CommandRecommendation, bool) {
	switch strings.TrimSpace(input) {
	case "/status":
		return CommandRecommendation{Route: CommandRouteStatus, Kind: CommandInputSlash}, true
	case "/review":
		return CommandRecommendation{Route: CommandRouteReview, Kind: CommandInputSlash}, true
	case "/help":
		return CommandRecommendation{Route: CommandRouteHelp, Kind: CommandInputSlash}, true
	case "/history":
		return CommandRecommendation{Route: CommandRouteHistory, Kind: CommandInputSlash}, true
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
	case "s":
		return CommandRecommendation{Route: CommandRouteStatus, Kind: CommandInputShortcut}, true
	case "i":
		return CommandRecommendation{Route: CommandRouteReview, Kind: CommandInputShortcut}, true
	case "h":
		return CommandRecommendation{Route: CommandRouteHistory, Kind: CommandInputShortcut}, true
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

package agents

// RouteKind is the gateway feature an SSH username resolves to.
type RouteKind int

const (
	// RouteInvalid means the username is not a usable agent name and is
	// not one of the reserved names either.
	RouteInvalid RouteKind = iota
	// RouteMenu serves the interactive Bubble Tea agent menu ("levi" or
	// "menu").
	RouteMenu
	// RouteNew serves the "create a new agent" TUI prompt ("new").
	RouteNew
	// RouteDirect attaches directly to the Sandbox named agent-<username>,
	// offering to create it first if it does not exist.
	RouteDirect
)

func (k RouteKind) String() string {
	switch k {
	case RouteMenu:
		return "menu"
	case RouteNew:
		return "new"
	case RouteDirect:
		return "direct"
	default:
		return "invalid"
	}
}

// Route describes the routing decision made for an SSH username.
type Route struct {
	Kind RouteKind
	// AgentName is set only for RouteDirect, and is the name used to build
	// the Sandbox name agent-<AgentName>.
	AgentName string
	// Err explains why RouteInvalid was chosen.
	Err error
}

// DecideRoute maps an SSH username to the gateway feature it selects. It is
// pure and side-effect free: it does not check whether the agent's Sandbox
// actually exists, only whether the username is routable at all.
func DecideRoute(username string) Route {
	switch username {
	case "levi", "menu":
		return Route{Kind: RouteMenu}
	case "new":
		return Route{Kind: RouteNew}
	}
	if err := ValidateName(username); err != nil {
		return Route{Kind: RouteInvalid, Err: err}
	}
	return Route{Kind: RouteDirect, AgentName: username}
}

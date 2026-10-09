package agents

// allowedForwardHosts are the only direct-tcpip destination hosts the
// gateway will tunnel: the forwarded connection is bridged into the
// Sandbox's network namespace via pods/portforward, so "localhost" there
// means localhost inside the pod, not on the gateway.
var allowedForwardHosts = map[string]bool{
	"localhost": true,
	"127.0.0.1": true,
	"::1":       true,
}

// AllowForwardDestination reports whether a direct-tcpip request's
// destination host may be forwarded. Only loopback destinations are
// allowed; anything else (another pod's IP, an external host, etc.) is
// rejected so the gateway can't be used as a general-purpose relay.
func AllowForwardDestination(host string) bool {
	return allowedForwardHosts[host]
}

// AllowForwardUser reports whether username may use local port forwarding
// at all. Only direct-agent usernames may forward; the reserved menu/new
// usernames have no single Sandbox to forward into.
func AllowForwardUser(username string) bool {
	return DecideRoute(username).Kind == RouteDirect
}

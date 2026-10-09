// Package agents holds pure, dependency-free logic for the agent-ssh
// gateway: name validation, Sandbox manifest rendering, SSH username
// routing decisions, and the port-forwarding destination allow-list. It has
// no knowledge of SSH or Kubernetes client libraries so it can be unit
// tested in isolation.
package agents

import (
	"fmt"
	"regexp"
)

// nameRE mirrors validate_name in services/agents/bin/agent: a DNS-1123
// label, lowercase letters/digits/hyphens, starting and ending with a
// letter or digit, at most 50 characters total.
var nameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,48}[a-z0-9])?$`)

// MaxNameLength is the longest permitted agent name (bin/agent's limit, so
// that "agent-<name>" and "home-agent-<name>" both stay within the
// Kubernetes 63-character object-name limit).
const MaxNameLength = 50

// ValidateName reports whether name is a usable agent name: a DNS-1123
// label of at most MaxNameLength characters.
func ValidateName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("name must not be empty")
	}
	if len(name) > MaxNameLength {
		return fmt.Errorf("name %q is too long: use at most %d characters", name, MaxNameLength)
	}
	if !nameRE.MatchString(name) {
		return fmt.Errorf("invalid name %q: use lowercase letters, digits, and hyphens; start and end with a letter or digit; at most %d characters", name, MaxNameLength)
	}
	return nil
}

// reservedNames are SSH usernames that select a gateway feature rather than
// an agent to attach to.
var reservedNames = map[string]bool{
	"levi": true,
	"menu": true,
	"new":  true,
}

// IsReserved reports whether username is a reserved gateway username (one
// that routes to a built-in feature rather than an agent of that name).
func IsReserved(username string) bool {
	return reservedNames[username]
}

// SandboxName returns the Sandbox object name for an agent name, matching
// sandbox_name() in bin/agent.
func SandboxName(name string) string {
	return "agent-" + name
}

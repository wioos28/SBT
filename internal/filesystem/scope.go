// Package filesystem implements SBT's scoped file manager and its SHA-256
// integrity helpers.
//
// A scope bounds every operation to one root. The default scope for the file
// manager is SANDBOX/WORKSPACE; the HOST scope exists but is read-only by
// default and can never be reached through the sandbox, so a command can never
// steer SBT's file manager at an arbitrary host path.
package filesystem

import (
	"fmt"
	"strings"
)

// Scope is a bounded filesystem view.
type Scope string

// The available scopes.
const (
	// Host is the host filesystem. It is read-only by default and is never the
	// default scope of an operation.
	Host Scope = "HOST"
	// Sandbox is the sandbox's own private tree.
	Sandbox Scope = "SANDBOX"
	// Workspace is the sandbox workspace copy under ~/.sbt/workspaces.
	Workspace Scope = "WORKSPACE"
	// Model is the model store under ~/.sbt/models.
	Model Scope = "MODEL"
)

// AllScopes lists the scopes in display order.
func AllScopes() []Scope { return []Scope{Sandbox, Workspace, Model, Host} }

// ParseScope decodes a scope name (case-insensitive).
func ParseScope(s string) (Scope, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "HOST":
		return Host, nil
	case "SANDBOX":
		return Sandbox, nil
	case "WORKSPACE":
		return Workspace, nil
	case "MODEL":
		return Model, nil
	default:
		return "", fmt.Errorf("unknown scope %q (want HOST, SANDBOX, WORKSPACE or MODEL)", s)
	}
}

// DefaultScope is what the file manager opens with.
func DefaultScope() Scope { return Sandbox }

// ReadOnly reports whether a scope refuses writes.
func (s Scope) ReadOnly() bool { return s == Host }

// Describe returns a one-line explanation of a scope.
func (s Scope) Describe() string {
	switch s {
	case Host:
		return "host filesystem (read-only, restricted)"
	case Sandbox:
		return "sandbox private filesystem"
	case Workspace:
		return "sandbox workspace copy"
	case Model:
		return "local model store"
	default:
		return string(s)
	}
}

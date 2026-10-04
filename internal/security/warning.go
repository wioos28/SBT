package security

import (
	"fmt"
	"strings"
	"time"

	"github.com/wioos28/sbt/internal/i18n"
)

// Alert is one security event: what happened, to which sandbox, and what SBT did
// about it.
type Alert struct {
	// At is when the event happened.
	At time.Time `json:"at"`
	// Level is the severity.
	Level Level `json:"level"`
	// Sandbox is the sandbox the event belongs to (empty for host-level events).
	Sandbox string `json:"sandbox,omitempty"`
	// Process is the process involved, when known.
	Process string `json:"process,omitempty"`
	// Event is a short machine-ish description.
	Event string `json:"event"`
	// Action is what SBT did in response, e.g. "process frozen".
	Action string `json:"action,omitempty"`
	// Result is the outcome of the action.
	Result string `json:"result,omitempty"`
	// Reason carries detail for the user.
	Reason string `json:"reason,omitempty"`
	// Disabled marks the sandbox as disabled as a result of this alert.
	Disabled bool `json:"disabled,omitempty"`
}

// WarningModal is the presentation form of an Alert: title, body lines and the
// buttons the UI offers.
type WarningModal struct {
	Alert   Alert
	Title   string
	Body    []string
	Buttons []string
	// Blocking is true for findings that must stop execution until the user
	// acts (CRITICAL isolation failures, DANGER operations).
	Blocking bool
	// AllowIgnore is deliberately false for critical isolation failures: SBT
	// does not offer an "Ignore" that would let a broken sandbox keep running.
	AllowIgnore bool
}

// NewWarningModal builds a WarningModal from an alert.
func NewWarningModal(a Alert) WarningModal {
	w := WarningModal{Alert: a}
	switch {
	case a.Level >= Critical:
		w.Title = i18n.T(i18n.KeyWarningCritical)
		w.Blocking = true
		w.AllowIgnore = false
		w.Buttons = []string{"Inspect", "Destroy"}
	case a.Level == Danger:
		w.Title = i18n.T(i18n.KeyWarningDanger)
		w.Blocking = true
		w.AllowIgnore = false
		w.Buttons = []string{"View Fix", "Cancel"}
	case a.Level == Warning:
		w.Title = i18n.T(i18n.KeyWarningWarning)
		w.AllowIgnore = true
		w.Buttons = []string{"View Fix", "Continue", "Cancel"}
	default:
		w.Title = a.Level.Translate()
		w.AllowIgnore = true
		w.Buttons = []string{"OK"}
	}
	if a.Sandbox != "" {
		w.Body = append(w.Body, "Sandbox: "+a.Sandbox)
	}
	if a.Reason != "" {
		w.Body = append(w.Body, "Reason: "+a.Reason)
	}
	if a.Action != "" {
		w.Body = append(w.Body, "Action: "+a.Action)
	}
	return w
}

// Modal renders a WarningModal as a centered ASCII modal. The layout is intentionally
// plain so it works in any terminal and in the web UI (as a <pre> block).
func (w WarningModal) Modal() string {
	const width = 54
	inner := width - 4

	lines := []string{}
	add := func(s string) {
		for _, piece := range wrap(s, inner) {
			lines = append(lines, piece)
		}
	}
	add(w.Title)
	lines = append(lines, "")
	for _, b := range w.Body {
		add(b)
	}
	if w.Alert.Level >= Critical {
		lines = append(lines, "")
		add("CRITICAL: sandbox activity is stopped or frozen.")
		if !w.AllowIgnore {
			add("There is no Ignore option for this failure.")
		}
	}
	if len(w.Buttons) > 0 {
		lines = append(lines, "")
		lines = append(lines, buttonRow(w.Buttons, inner))
	}

	var b strings.Builder
	border := "+" + strings.Repeat("-", width-2) + "+"
	blank := "|" + strings.Repeat(" ", width-2) + "|"
	b.WriteString(border + "\n")
	b.WriteString(blank + "\n")
	for _, ln := range lines {
		pad := inner - len(ln)
		if pad < 0 {
			pad = 0
		}
		b.WriteString("|  " + ln + strings.Repeat(" ", pad) + "  |\n")
	}
	b.WriteString(blank + "\n")
	b.WriteString(border)
	return b.String()
}

func buttonRow(buttons []string, inner int) string {
	parts := make([]string, 0, len(buttons))
	for _, b := range buttons {
		parts = append(parts, "[ "+b+" ]")
	}
	row := strings.Join(parts, "  ")
	if len(row) > inner {
		// Drop to a single-line fallback rather than overflowing the box.
		row = strings.Join(buttons, " ")
	}
	return row
}

func wrap(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	if len(s) <= width {
		return []string{s}
	}
	var out []string
	words := strings.Fields(s)
	line := ""
	for _, wd := range words {
		if line == "" {
			line = wd
			continue
		}
		if len(line)+1+len(wd) > width {
			out = append(out, line)
			line = wd
			continue
		}
		line += " " + wd
	}
	if line != "" {
		out = append(out, line)
	}
	if len(out) == 0 {
		out = []string{s}
	}
	return out
}

// String renders an alert as one timeline line.
func (a Alert) String() string {
	ts := a.At.Format("15:04:05")
	if a.At.IsZero() {
		ts = "--:--:--"
	}
	sb := a.Sandbox
	if sb == "" {
		sb = "-"
	}
	line := fmt.Sprintf("%s %-8s %-16s %s", ts, a.Level.String(), sb, a.Event)
	if a.Action != "" {
		line += " -> " + a.Action
	}
	if a.Result != "" {
		line += " (" + a.Result + ")"
	}
	return line
}

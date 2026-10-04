package tui

import (
	"strings"
	"time"
)

// The Permissions view.
//
// It answers one question - what is SBT actually allowed to do here - with the
// evidence behind every answer. A row is never a claim: the session fills it
// from a measured capability report, and a verdict that could not be measured
// reads UNKNOWN rather than being rounded up to allowed.

// PermissionRow is one measured capability.
type PermissionRow struct {
	Name string
	// Status is the verdict word: ALLOWED, LIMITED, DENIED or UNKNOWN.
	Status string
	State  StateKind
	Reason string
	// Feature is what in SBT needs the capability.
	Feature string
	// Remedy is the least-privilege fix, or empty when none is needed.
	Remedy string
}

// PermissionReport is the whole capability table.
type PermissionReport struct {
	Rows []PermissionRow
	At   time.Time
}

// Counts summarises the report by verdict.
func (r PermissionReport) Counts() (allowed, limited, denied int) {
	for _, row := range r.Rows {
		switch row.State {
		case StateOK:
			allowed++
		case StateWarn:
			limited++
		case StateDanger:
			denied++
		}
	}
	return
}

// permissionState maps a capability verdict onto a severity. UNKNOWN is muted
// on purpose: it is an absence of information, not a failure.
func permissionState(status string) StateKind {
	switch strings.ToUpper(status) {
	case "ALLOWED":
		return StateOK
	case "LIMITED":
		return StateWarn
	case "DENIED":
		return StateDanger
	default:
		return StateMeta
	}
}

// permissionsView draws the capability table.
func (i *Interpreter) permissionsView(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	p := t.Palette
	rep := s.Permissions
	allowed, limited, denied := rep.Counts()
	right := itoa(allowed) + " ok  " + itoa(limited) + " limited  " + itoa(denied) + " denied"
	inner := t.Panel(b, r.X, r.Y, r.W, r.H, tr("perms.title", "permissions"), right, p.Border, st.View == ViewPermissions)
	if inner.Empty() {
		return
	}
	if len(rep.Rows) == 0 {
		i.emptyState(b, inner, tr("perms.none", "no permission report yet"), tr("perms.hint", "the session measures the host at startup"))
		return
	}
	y := inner.Y
	for _, row := range rep.Rows {
		if y >= inner.Bottom() {
			break
		}
		b.WriteClipped(inner.X, y, inner.Right(), Pad(row.Name, 20), Style{Fg: p.Text, Bold: true})
		col := inner.X + 20
		sty := Style{Fg: p.Text}
		switch row.State {
		case StateOK:
			sty.Fg = p.Green
		case StateWarn:
			sty.Fg = p.Yellow
		case StateDanger:
			sty.Fg = p.Red
		default:
			sty.Fg = p.Muted
		}
		b.WriteClipped(col, y, inner.Right(), row.Status, sty)
		y++
		// The evidence line is always drawn: a verdict without the reason that
		// produced it is an opinion, and this view exists to stop those.
		if row.Reason != "" && y < inner.Bottom() {
			b.WriteClipped(inner.X+2, y, inner.Right(), row.Reason, Style{Fg: p.Muted})
			y++
		}
		if row.Feature != "" && y < inner.Bottom() {
			b.WriteClipped(inner.X+2, y, inner.Right(), tr("perms.needs", "needs: ")+row.Feature, Style{Fg: p.Muted})
			y++
		}
		if row.Remedy != "" && y < inner.Bottom() {
			b.WriteClipped(inner.X+2, y, inner.Right(), tr("perms.least", "least privilege: ")+row.Remedy, Style{Fg: p.Info})
			y++
		}
		if y < inner.Bottom() {
			y++
		}
	}
}

package tui

// The permission prompt.
//
// The rule that shapes this file: choosing "Allow" never grants a capability.
// SBT cannot give itself privileges it does not have, and a dialog implying
// otherwise would be the most dangerous thing in the program. What the choice
// does is record that the user has been told what is missing, why, and at what
// risk - and then let the session attempt the work. The session still refuses
// anything it cannot verify and still says exactly why. "Allow for the
// session" means "stop asking", never "ask the kernel for more".

// PermissionChoice is the selected row of the prompt.
type PermissionChoice int

// The four answers, in the order they are drawn. Escape always cancels.
const (
	PermAllowOnce PermissionChoice = iota
	PermAllowSession
	PermConfigure
	PermCancel
)

// PermissionLabels are the four answers as the product specifies them.
var PermissionLabels = [4]string{
	"[ Allow once ]",
	"[ Allow session ]",
	"[ Configure ]",
	"[ Cancel ]",
}

// PermissionState is a pending PERMISSION REQUIRED dialog.
type PermissionState struct {
	Open bool
	// Capability names the exact thing that is missing.
	Capability string
	// Reason says which SBT feature needs it.
	Reason string
	// Risk is the short, honest consequence of proceeding without it.
	Risk string
	// Detail is the measured evidence behind the refusal.
	Detail string
	Choice PermissionChoice
	// Argv is the work the user was trying to start, so an approval applies to
	// the command they actually typed rather than to whatever comes next.
	Argv []string
}

// openPermission raises the prompt for a blocked action.
func (st *UIState) openPermission(capability, reason, risk, detail string, argv []string) event {
	st.Permission = PermissionState{
		Open:       true,
		Capability: capability,
		Reason:     reason,
		Risk:       risk,
		Detail:     detail,
		Choice:     PermCancel,
		Argv:       argv,
	}
	return event{kind: evNone}
}

// permissionKey drives the prompt: up and down move, enter confirms, escape
// cancels. The prompt never falls through to a view's own keys, so a stray rune
// cannot change the answer.
func (st *UIState) permissionKey(k Key, snap *Snapshot) event {
	switch k.Type {
	case KeyEsc:
		st.Permission = PermissionState{}
		st.flash("cancelled - nothing was changed", StateMeta, snap.Now)
		return event{kind: evNone}
	case KeyUp:
		if st.Permission.Choice > 0 {
			st.Permission.Choice--
		}
		return event{kind: evNone}
	case KeyDown:
		if st.Permission.Choice < PermCancel {
			st.Permission.Choice++
		}
		return event{kind: evNone}
	case KeyEnter:
		p := st.Permission
		st.Permission = PermissionState{}
		switch p.Choice {
		case PermAllowOnce, PermAllowSession:
			// The acknowledgement is what travels. The session still decides.
			st.notePermissionAnswer(p.Choice)
			return event{kind: evRun, argv: p.Argv, scope: p.Choice}
		case PermConfigure:
			st.SetView(ViewPermissions, snap.Now)
			st.Focus = focusWorkspace
		default:
			st.flash("cancelled - nothing was changed", StateMeta, snap.Now)
		}
		return event{kind: evNone}
	}
	return event{kind: evNone}
}

// drawPermission renders the prompt. It is drawn last, above every other
// overlay, because it is the one question SBT refuses to answer for the user.
func (i *Interpreter) drawPermission(b *Buffer, st *UIState) {
	p := st.Permission
	t := i.Theme
	pal := t.Palette
	w := 62
	if st.Width-4 < w {
		w = st.Width - 4
	}
	if w < 26 {
		return
	}
	h := 10
	if p.Detail != "" {
		h++
	}
	x := (st.Width - w) / 2
	y := (st.Height - h) / 2
	if y < 1 {
		y = 1
	}
	inner := t.Panel(b, x, y, w, h, "PERMISSION REQUIRED", "esc = cancel", pal.Warning, true)
	if inner.Empty() {
		return
	}
	y0 := inner.Y
	n := 0
	put := func(label, value string, sty Style) {
		if y0+n >= inner.Bottom() {
			return
		}
		b.WriteClipped(inner.X, y0+n, inner.Right(), Pad(label, 12), Style{Fg: pal.Muted})
		b.WriteClipped(inner.X+12, y0+n, inner.Right(), value, sty)
		n++
	}
	put("Capability", Truncate(p.Capability, inner.W-14), Style{Fg: pal.Text, Bold: true})
	put("Reason", Truncate(p.Reason, inner.W-14), Style{Fg: pal.Text})
	put("Risk", Truncate(p.Risk, inner.W-14), Style{Fg: pal.Warning})
	if p.Detail != "" {
		put("Evidence", Truncate(p.Detail, inner.W-14), Style{Fg: pal.Muted})
	}
	n++
	for j, label := range PermissionLabels {
		if y0+n >= inner.Bottom() {
			break
		}
		sty := Style{Fg: pal.Muted}
		if PermissionChoice(j) == p.Choice {
			sty = Style{Fg: pal.Bg, Bg: pal.Primary, HasBg: true, Bold: true}
		}
		b.WriteClipped(inner.X+2, y0+n, inner.Right(), label, sty)
		n++
	}
}

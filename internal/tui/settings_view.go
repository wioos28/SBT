package tui

import (
	"fmt"

	"github.com/wioos28/sbt/internal/config"
)

// The Settings centre.
//
// Every row is driven from the same declared schema the CLI uses, so the
// Settings view and `sbt setting` can never disagree about what exists, what a
// value means or which values are legal. The view never writes the file: it
// emits a request and the session validates, stores and applies the change,
// which is what makes an edit take effect on the very next frame.
//
// Navigation is keyboard only: left and right switch section, up and down move
// the cursor, enter edits or toggles, escape goes back.

type settingKind int

const (
	settingToggle settingKind = iota
	settingChoice
	settingNumber
	settingText
	settingAction
)

type settingRow struct {
	Key   string
	Label string
	Kind  settingKind
}

type settingSection struct {
	Title string
	Rows  []settingRow
}

// openPermissions is the Permissions row: it switches view rather than storing
// a value, which is why rows can be an action as well as a setting.
var openPermissions = settingRow{
	Key:   "@permissions",
	Label: tr("settings.row.permissions", "Open permissions centre"),
	Kind:  settingAction,
}

// settingSections is the Settings layout. The order is the order a user thinks
// in: how it looks, what it says, how it moves, what it may do.
var settingSections = []settingSection{
	{Title: tr("settings.section.appearance", "Appearance"), Rows: []settingRow{
		{Key: "ui.palette", Label: tr("settings.row.palette", "Palette preset"), Kind: settingChoice},
		{Key: "ui.theme", Label: tr("settings.row.theme", "Ground"), Kind: settingChoice},
		{Key: "ui.welcome", Label: tr("settings.row.welcome", "Welcome screen"), Kind: settingToggle},
		{Key: "ui.files_row", Label: tr("settings.row.filesRow", "File row"), Kind: settingToggle},
		{Key: "ui.compact", Label: tr("settings.row.compact", "Compact layout"), Kind: settingToggle},
	}},
	{Title: tr("settings.section.colors", "Colors"), Rows: []settingRow{
		{Key: "color.primary", Label: tr("settings.row.colorPrimary", "Primary"), Kind: settingText},
		{Key: "color.accent", Label: tr("settings.row.colorAccent", "Accent"), Kind: settingText},
		{Key: "color.info", Label: tr("settings.row.colorInfo", "Info"), Kind: settingText},
	}},
	{Title: tr("settings.section.animations", "Animations"), Rows: []settingRow{
		{Key: "anim.startup", Label: tr("settings.row.animStartup", "Startup animation"), Kind: settingToggle},
		{Key: "anim.typing", Label: tr("settings.row.animTyping", "Typing animation"), Kind: settingToggle},
		{Key: "anim.typing_speed", Label: tr("settings.row.animTypingSpeed", "Typing speed"), Kind: settingChoice},
		{Key: "anim.typing_intensity", Label: tr("settings.row.animIntensity", "Sweep intensity"), Kind: settingNumber},
		{Key: "anim.typing_color", Label: tr("settings.row.animTypingColor", "Sweep colour"), Kind: settingText},
		{Key: "anim.menu", Label: tr("settings.row.animMenu", "Menu transitions"), Kind: settingToggle},
		{Key: "anim.glow", Label: tr("settings.row.animGlow", "Glow"), Kind: settingToggle},
		{Key: "anim.warnings", Label: tr("settings.row.animWarnings", "Warning animation"), Kind: settingToggle},
	}},
	{Title: tr("settings.section.terminal", "Terminal"), Rows: []settingRow{
		{Key: "general.default_memory", Label: tr("settings.row.memory", "Memory limit (MB)"), Kind: settingNumber},
		{Key: "general.default_network", Label: tr("settings.row.network", "Network policy"), Kind: settingChoice},
	}},
	{Title: tr("settings.section.security", "Security"), Rows: []settingRow{
		{Key: "security.require_isolation", Label: tr("settings.row.requireIsolation", "Require verified isolation"), Kind: settingToggle},
		{Key: "security.read_only_host", Label: tr("settings.row.readOnlyHost", "Read-only host trees"), Kind: settingToggle},
		{Key: "security.environment_isolation", Label: tr("settings.row.envIsolation", "Scrub environment"), Kind: settingToggle},
		{Key: "security.confirm_dangerous", Label: tr("settings.row.confirmDangerous", "Confirm dangerous actions"), Kind: settingToggle},
		openPermissions,
	}},
	{Title: tr("settings.section.language", "Language"), Rows: []settingRow{
		{Key: "language.locale", Label: tr("settings.row.locale", "Interface language"), Kind: settingChoice},
	}},
	{Title: tr("settings.section.troll", "Troll"), Rows: []settingRow{
		{Key: "troll.enabled", Label: tr("settings.row.trollOn", "Fun messages"), Kind: settingToggle},
		{Key: "troll.frequency", Label: tr("settings.row.trollFreq", "Frequency (0-100)"), Kind: settingNumber},
		{Key: "troll.intensity", Label: tr("settings.row.trollIntensity", "Intensity (0-100)"), Kind: settingNumber},
		{Key: "ui.pet", Label: tr("settings.row.pet", "Companion"), Kind: settingChoice},
	}},
	{Title: tr("settings.section.advanced", "Advanced"), Rows: []settingRow{
		{Key: "notifications.enabled", Label: tr("settings.row.notifyOn", "Notifications"), Kind: settingToggle},
		{Key: "notifications.minimum_level", Label: tr("settings.row.notifyLevel", "Minimum level"), Kind: settingChoice},
		{Key: "lan.enabled", Label: tr("settings.row.lanOn", "LAN panel"), Kind: settingToggle},
		{Key: "lan.port", Label: tr("settings.row.lanPort", "LAN port"), Kind: settingNumber},
		{Key: "lan.authentication", Label: tr("settings.row.lanAuth", "LAN authentication"), Kind: settingToggle},
	}},
}

// SettingsState is the cursor of the Settings view.
type SettingsState struct {
	Section int
	Cursor  int
	Scroll  int
	Editing bool
	Draft   string
	Message string
}

// current returns the section and row the cursor is on.
func (ss *SettingsState) current() (settingSection, settingRow, bool) {
	if ss.Section < 0 || ss.Section >= len(settingSections) {
		return settingSection{}, settingRow{}, false
	}
	sec := settingSections[ss.Section]
	if ss.Cursor < 0 || ss.Cursor >= len(sec.Rows) {
		return sec, settingRow{}, false
	}
	return sec, sec.Rows[ss.Cursor], true
}

// clamp keeps the cursor inside the section.
func (ss *SettingsState) clamp() {
	if ss.Section < 0 {
		ss.Section = 0
	}
	if ss.Section >= len(settingSections) {
		ss.Section = len(settingSections) - 1
	}
	sec := settingSections[ss.Section]
	if ss.Cursor >= len(sec.Rows) {
		ss.Cursor = len(sec.Rows) - 1
	}
	if ss.Cursor < 0 {
		ss.Cursor = 0
	}
}

// settingValueLabel renders a stored value for the value column.
func settingValueLabel(key string, raw any) string {
	switch v := raw.(type) {
	case bool:
		return trOnOff(v)
	case int:
		return itoa(v)
	case int64:
		return itoa(int(v))
	case string:
		if v == "" {
			return "(unset)"
		}
		return v
	case nil:
		return "(unset)"
	default:
		return fmt.Sprint(raw)
	}
}

// validateSetting turns typed text into a storable value, refusing anything the
// schema does not declare or does not allow. Returning an error here means the
// value is never written, so an out-of-range number cannot reach the config.
func validateSetting(key, raw string) (any, error) {
	def, ok := config.SchemaKey(key)
	if !ok {
		return nil, fmt.Errorf("unknown setting %q", key)
	}
	val, err := config.Coerce(key, raw)
	if err != nil {
		return nil, fmt.Errorf("not a valid value")
	}
	if def.Kind == config.KindInt {
		n, _ := val.(int)
		if lo, hi, ranged := config.IntRange(key); ranged && (n < lo || n > hi) {
			return nil, fmt.Errorf("must be between %d and %d", lo, hi)
		}
	}
	if _, isColour := ParseHex(raw); def.Kind == config.KindString && isHexSetting(key) && !isColour {
		return nil, fmt.Errorf("not a #RRGGBB colour")
	}
	if choices := config.ChoiceValues(key); choices != nil {
		ok := false
		for _, c := range choices {
			if c == raw {
				ok = true
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("must be one of: %s", joinChoices(choices))
		}
	}
	return val, nil
}

func isHexSetting(key string) bool {
	for _, k := range []string{"color.primary", "color.accent", "color.info", "anim.typing_color"} {
		if k == key {
			return true
		}
	}
	return false
}

func joinChoices(choices []string) string {
	out := ""
	for i, c := range choices {
		if i > 0 {
			out += " "
		}
		out += c
	}
	return out
}

// settingsKey handles the Settings view. Editing is inline and validated before
// anything is stored; escape always returns to the terminal, never deeper.
func (st *UIState) settingsKey(k Key, snap *Snapshot) event {
	ss := &st.Settings
	ss.clamp()
	if ss.Editing {
		return st.settingsEditKey(k, snap)
	}
	sec := settingSections[ss.Section]
	switch k.Type {
	case KeyEsc:
		st.SetView(ViewTerminal, snap.Now)
		st.Focus = focusInput
		return event{kind: evNone}
	case KeyLeft:
		if ss.Section > 0 {
			ss.Section--
			ss.Cursor = 0
		}
		return event{kind: evNone}
	case KeyRight:
		if ss.Section < len(settingSections)-1 {
			ss.Section++
			ss.Cursor = 0
		}
		return event{kind: evNone}
	case KeyUp:
		if ss.Cursor > 0 {
			ss.Cursor--
		}
		return event{kind: evNone}
	case KeyDown:
		if ss.Cursor < len(sec.Rows)-1 {
			ss.Cursor++
		}
		return event{kind: evNone}
	case KeyEnter:
		return st.settingsActivate(snap)
	}
	return event{kind: evNone}
}

// settingsActivate toggles, cycles, edits, or switches view for the current row.
func (st *UIState) settingsActivate(snap *Snapshot) event {
	_, row, ok := st.Settings.current()
	if !ok {
		return event{kind: evNone}
	}
	if row.Kind == settingAction {
		st.SetView(ViewPermissions, snap.Now)
		return event{kind: evNone}
	}
	cur := snap.Settings[row.Key]
	if row.Kind == settingToggle {
		on, _ := cur.(bool)
		return event{kind: evSetSetting, skey: row.Key, sval: !on}
	}
	if row.Kind == settingChoice {
		choices := config.ChoiceValues(row.Key)
		if len(choices) == 0 {
			return event{kind: evNone}
		}
		next := choices[0]
		for i, c := range choices {
			if c == settingValueLabel(row.Key, cur) && i+1 < len(choices) {
				next = choices[i+1]
			}
		}
		return event{kind: evSetSetting, skey: row.Key, sval: next}
	}
	st.Settings.Editing = true
	st.Settings.Draft = settingValueLabel(row.Key, cur)
	st.Settings.Message = ""
	return event{kind: evNone}
}

// settingsEditKey handles the inline editor for numbers and free text.
func (st *UIState) settingsEditKey(k Key, snap *Snapshot) event {
	ss := &st.Settings
	_, row, ok := ss.current()
	if !ok {
		ss.Editing = false
		return event{kind: evNone}
	}
	switch k.Type {
	case KeyEsc:
		ss.Editing = false
		ss.Draft = ""
		ss.Message = ""
		return event{kind: evNone}
	case KeyBackspace:
		if r := []rune(ss.Draft); len(r) > 0 {
			ss.Draft = string(r[:len(r)-1])
		}
		return event{kind: evNone}
	case KeyEnter:
		val, err := validateSetting(row.Key, ss.Draft)
		if err != nil {
			ss.Message = tr("settings.notSaved", "not saved") + ": " + err.Error()
			return event{kind: evNone}
		}
		ss.Editing = false
		ss.Draft = ""
		ss.Message = tr("settings.saved", "saved")
		return event{kind: evSetSetting, skey: row.Key, sval: val}
	}
	if k.Type == KeyRune && !k.Ctrl && !k.Alt {
		ss.Draft += string(k.Rune)
		ss.Message = ""
	}
	return event{kind: evNone}
}

// permissionsKey handles the Permissions view. It is a read-only report, so
// there is nothing to edit here: escape returns, and the list is scrollable.
func (st *UIState) permissionsKey(k Key, snap *Snapshot) event {
	switch k.Type {
	case KeyEsc:
		st.SetView(ViewTerminal, snap.Now)
		st.Focus = focusInput
		return event{kind: evNone}
	case KeyUp:
		if st.List.Index > 0 {
			st.List.Index--
		}
		return event{kind: evNone}
	case KeyDown:
		if st.List.Index < len(snap.Permissions.Rows)-1 {
			st.List.Index++
		}
		return event{kind: evNone}
	}
	return event{kind: evNone}
}

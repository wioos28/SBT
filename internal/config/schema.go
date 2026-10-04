package config

import (
	"strconv"
	"strings"
)

// ValueKind is the type of a setting value.
type ValueKind string

// Supported value kinds.
const (
	KindBool   ValueKind = "bool"
	KindInt    ValueKind = "int"
	KindString ValueKind = "string"
)

// Definition declares one setting: its key, default, type and the UI category it
// belongs to. Declaring settings in one place is what lets `sbt setting`, the TUI
// Settings screen and the LAN panel agree on what exists.
type Definition struct {
	Key      string
	Category string
	Kind     ValueKind
	Default  any
	Secret   bool
	Help     string
}

// Categories shown by the Settings UI, in display order.
const (
	CatGeneral       = "General"
	CatAI            = "AI"
	CatCLI           = "CLI"
	CatSecurity      = "Security"
	CatLanguage      = "Language"
	CatAppearance    = "Appearance"
	CatColors        = "Colors"
	CatAnimations    = "Animations"
	CatTroll         = "Troll"
	CatLAN           = "LAN"
	CatNotifications = "Notifications"
)

// Schema is the canonical list of settings.
var Schema = []Definition{
	{Key: "general.default_memory", Category: CatGeneral, Kind: KindInt, Default: 512, Help: "memory limit (MB) for a new sandbox"},
	{Key: "general.default_network", Category: CatGeneral, Kind: KindString, Default: "off", Help: "default network policy: off or on"},
	{Key: "general.workspace", Category: CatGeneral, Kind: KindString, Default: "~", Help: "default host project copied into a sandbox workspace"},

	{Key: "ai.default_model", Category: CatAI, Kind: KindString, Default: "qwen3-0.6b", Help: "model used when none is selected"},
	{Key: "ai.default_runtime", Category: CatAI, Kind: KindString, Default: "llama.cpp", Help: "inference runtime SBT drives"},
	{Key: "ai.sandbox", Category: CatAI, Kind: KindBool, Default: true, Help: "run local AI inside the SBT sandbox"},

	{Key: "cli.auto_detect", Category: CatCLI, Kind: KindBool, Default: true, Help: "detect installed AI coding CLIs automatically"},
	{Key: "cli.sandbox", Category: CatCLI, Kind: KindBool, Default: true, Help: "run AI coding CLIs inside the SBT sandbox by default"},

	{Key: "security.require_isolation", Category: CatSecurity, Kind: KindBool, Default: true, Help: "refuse to run when isolation cannot be verified (fail closed)"},
	{Key: "security.read_only_host", Category: CatSecurity, Kind: KindBool, Default: true, Help: "expose host trees read-only inside the sandbox"},
	{Key: "security.environment_isolation", Category: CatSecurity, Kind: KindBool, Default: true, Help: "scrub the environment inside the sandbox"},
	{Key: "security.confirm_dangerous", Category: CatSecurity, Kind: KindBool, Default: true, Help: "ask before dangerous or destructive operations"},

	{Key: "ui.show_prefix", Category: CatAppearance, Kind: KindBool, Default: true, Help: "prefix command output with SBT>"},
	{Key: "ui.theme", Category: CatAppearance, Kind: KindString, Default: "dark", Help: "colour theme: dark, light or plain"},
	{Key: "ui.palette", Category: CatAppearance, Kind: KindString, Default: "sbt", Help: "palette preset: sbt, cyber, minimal, ocean, mono"},
	{Key: "ui.compact", Category: CatAppearance, Kind: KindBool, Default: false, Help: "compact layout"},
	{Key: "ui.animations", Category: CatAppearance, Kind: KindBool, Default: true, Help: "progress animations"},
	{Key: "ui.welcome", Category: CatAppearance, Kind: KindBool, Default: true, Help: "show the welcome screen after startup"},
	{Key: "ui.files_row", Category: CatAppearance, Kind: KindBool, Default: true, Help: "show the file row under the terminal"},

	{Key: "color.primary", Category: CatColors, Kind: KindString, Default: "#FF8A00", Help: "primary (brand) colour, HEX"},
	{Key: "color.accent", Category: CatColors, Kind: KindString, Default: "#00D9FF", Help: "accent colour for focus and typing, HEX"},
	{Key: "color.info", Category: CatColors, Kind: KindString, Default: "#00D9FF", Help: "informational / sweep colour, HEX"},

	{Key: "anim.startup", Category: CatAnimations, Kind: KindBool, Default: true, Help: "startup and loading animation"},
	{Key: "anim.typing", Category: CatAnimations, Kind: KindBool, Default: true, Help: "typing sweep on the command input"},
	{Key: "anim.typing_speed", Category: CatAnimations, Kind: KindString, Default: "auto", Help: "typing animation speed: auto, slow, normal, fast"},
	{Key: "anim.typing_intensity", Category: CatAnimations, Kind: KindInt, Default: 70, Help: "typing sweep intensity, 0-100"},
	{Key: "anim.typing_color", Category: CatAnimations, Kind: KindString, Default: "#00D9FF", Help: "typing sweep colour, HEX"},
	{Key: "anim.menu", Category: CatAnimations, Kind: KindBool, Default: true, Help: "menu and view transitions"},
	{Key: "anim.glow", Category: CatAnimations, Kind: KindBool, Default: true, Help: "glow on live elements"},
	{Key: "anim.warnings", Category: CatAnimations, Kind: KindBool, Default: true, Help: "warning and critical alert animation"},

	{Key: "troll.enabled", Category: CatTroll, Kind: KindBool, Default: false, Help: "harmless cosmetic jokes (never touches security UI)"},
	{Key: "troll.frequency", Category: CatTroll, Kind: KindInt, Default: 15, Help: "how often a troll message appears, 0-100"},
	{Key: "troll.intensity", Category: CatTroll, Kind: KindInt, Default: 30, Help: "troll message boldness, 0-100"},

	{Key: "language.locale", Category: CatLanguage, Kind: KindString, Default: "en-US", Help: "active interface language"},

	{Key: "lan.enabled", Category: CatLAN, Kind: KindBool, Default: false, Help: "enable the LAN control panel (off by default)"},
	{Key: "lan.port", Category: CatLAN, Kind: KindInt, Default: 8787, Help: "port the LAN control panel listens on"},
	{Key: "lan.authentication", Category: CatLAN, Kind: KindBool, Default: true, Help: "require a session token for the LAN panel"},

	{Key: "notifications.enabled", Category: CatNotifications, Kind: KindBool, Default: true, Help: "show warnings and notifications"},
	{Key: "notifications.minimum_level", Category: CatNotifications, Kind: KindString, Default: "warning", Help: "lowest level that raises a notification"},
}

// SchemaKey returns the definition for a key.
func SchemaKey(key string) (Definition, bool) {
	for _, d := range Schema {
		if d.Key == key {
			return d, true
		}
	}
	return Definition{}, false
}

// Categories returns the declared categories in display order.
func Categories() []string {
	return []string{CatGeneral, CatAI, CatCLI, CatSecurity, CatLanguage, CatAppearance, CatColors, CatAnimations, CatTroll, CatLAN, CatNotifications}
}

// ByCategory returns the definitions of one category.
func ByCategory(cat string) []Definition {
	var out []Definition
	for _, d := range Schema {
		if d.Category == cat {
			out = append(out, d)
		}
	}
	return out
}

// IsSecret reports whether a key looks like a credential that must never be
// printed in normal output.
func IsSecret(key string) bool {
	k := strings.ToLower(key)
	for _, marker := range []string{"token", "secret", "password", "passwd", "credential", "apikey", "api_key", "private_key"} {
		if strings.Contains(k, marker) {
			return true
		}
	}
	return false
}

// Redact masks a value for display when the key looks secret.
func Redact(key string, value any) string {
	if IsSecret(key) {
		return "******** (redacted)"
	}
	return Render(value)
}

// Render formats a value for display.
func Render(value any) string { return formatScalar(value) }

// Coerce converts a textual value into the type declared for key.
func Coerce(key, raw string) (any, error) {
	def, ok := SchemaKey(key)
	if !ok {
		return nil, &UnknownKeyError{Key: key}
	}
	switch def.Kind {
	case KindBool:
		return strings.EqualFold(raw, "true") || raw == "1" || strings.EqualFold(raw, "on"), nil
	case KindInt:
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return nil, err
		}
		return n, nil
	default:
		return raw, nil
	}
}

// UnknownKeyError is returned when a caller references a key not in the schema.
type UnknownKeyError struct{ Key string }

func (e *UnknownKeyError) Error() string { return "unknown setting " + e.Key }

// IntRange is the inclusive valid range for an int setting. A zero Max means the
// key is not range checked; it is returned for the Settings UI so it can refuse
// an out-of-range value before it is stored.
func IntRange(key string) (min, max int, ok bool) {
	switch key {
	case "anim.typing_intensity", "troll.frequency", "troll.intensity":
		return 0, 100, true
	case "lan.port":
		return 1, 65535, true
	case "general.default_memory":
		return 16, 1 << 20, true
	}
	return 0, 0, false
}

// ChoiceValues lists the allowed values of a choice setting, or nil when the key
// is free-form text. The Settings UI cycles through these with the arrow keys.
func ChoiceValues(key string) []string {
	switch key {
	case "ui.theme":
		return []string{"dark", "light", "plain"}
	case "ui.palette":
		return []string{"sbt", "cyber", "minimal", "ocean", "mono"}
	case "anim.typing_speed":
		return []string{"auto", "slow", "normal", "fast"}
	case "general.default_network":
		return []string{"off", "on"}
	case "notifications.minimum_level":
		return []string{"info", "notice", "warning", "danger", "critical"}
	case "language.locale":
		return []string{"en-US", "vi-VN", "ru-RU", "zh-CN"}
	}
	return nil
}

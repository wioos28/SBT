// Package i18n provides the translation layer used by the TUI, the CLI and the
// LAN web panel. User-facing strings are looked up by key (i18n.T) so no string
// is hard-coded across the Go source.
//
// Translation files live in ~/.sbt/locales/<locale>.json and are plain JSON
// objects mapping keys to strings. A user can add a language by dropping a file
// in that directory or by `sbt language import <file>`.
package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FallbackLocale is the language used when a key is missing everywhere else.
const FallbackLocale = "en-US"

// Bundle holds every loaded locale and the currently selected one.
type Bundle struct {
	current string
	store   map[string]map[string]string
	dir     string
}

// NewBundle builds a bundle preloaded with the built-in languages.
func NewBundle() *Bundle {
	b := &Bundle{current: FallbackLocale, store: map[string]map[string]string{}}
	for _, locale := range BuiltinLocales() {
		b.LoadJSON(locale, defaultStrings(locale))
		if extra := uiStrings(locale); extra != nil {
			b.LoadJSON(locale, extra)
		}
	}
	return b
}

// LocaleDir returns the directory custom locales are stored in.
func LocaleDir() string {
	if v := os.Getenv("SBT_HOME"); v != "" {
		return filepath.Join(v, "locales")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".sbt", "locales")
	}
	return filepath.Join(home, ".sbt", "locales")
}

// BuiltinLocales lists the languages shipped with SBT.
func BuiltinLocales() []string { return []string{"en-US", "vi-VN", "ru-RU", "zh-CN"} }

// LoadDir loads every *.json file in dir as a locale named after the file.
func (b *Bundle) LoadDir(dir string) error {
	b.dir = dir
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		_ = b.ImportJSON(filepath.Join(dir, e.Name()))
	}
	return nil
}

// SetLanguage selects the locale used for lookups.
func (b *Bundle) SetLanguage(locale string) error {
	if locale == "" {
		locale = FallbackLocale
	}
	if !ValidLocale(locale) {
		return &InvalidLocaleError{Locale: locale}
	}
	if _, ok := b.store[locale]; !ok {
		b.store[locale] = map[string]string{}
	}
	b.current = locale
	return nil
}

// Current returns the active locale.
func (b *Bundle) Current() string {
	if b == nil {
		return FallbackLocale
	}
	return b.current
}

// T looks up a key in the active locale and falls back to en-US, then to the key
// itself so a missing translation is visible but never crashes the UI.
func (b *Bundle) T(key string) string {
	if b == nil {
		return key
	}
	if v, ok := b.store[b.current][key]; ok && v != "" {
		return v
	}
	if v, ok := b.store[FallbackLocale][key]; ok && v != "" {
		return v
	}
	return key
}

// Tf formats a translation with positional arguments.
func (b *Bundle) Tf(key string, args ...any) string { return sprintf(b.T(key), args...) }

// Has reports whether the active locale defines a key.
func (b *Bundle) Has(key string) bool {
	if b == nil {
		return false
	}
	_, ok := b.store[b.current][key]
	return ok
}

// LoadJSON merges values into a locale.
func (b *Bundle) LoadJSON(locale string, values map[string]string) {
	if b == nil {
		return
	}
	if b.store == nil {
		b.store = map[string]map[string]string{}
	}
	if _, ok := b.store[locale]; !ok {
		b.store[locale] = map[string]string{}
	}
	for k, v := range values {
		b.store[locale][k] = v
	}
}

// ImportJSON loads a locale JSON file. The file name (without extension) is the
// locale name.
func (b *Bundle) ImportJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return &InvalidLocaleError{Locale: filepath.Base(path), Reason: err.Error()}
	}
	locale := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if !ValidLocale(locale) {
		return &InvalidLocaleError{Locale: locale}
	}
	b.LoadJSON(locale, values)
	return nil
}

// ExportJSON writes the active locale to a JSON file.
func (b *Bundle) ExportJSON(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b.store[b.current], "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// ExportLocale writes a specific locale to disk.
func (b *Bundle) ExportLocale(locale, path string) error {
	if _, ok := b.store[locale]; !ok {
		return &InvalidLocaleError{Locale: locale, Reason: "not loaded"}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b.store[locale], "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// Languages returns the available locales, sorted.
func (b *Bundle) Languages() []string {
	if b == nil || len(b.store) == 0 {
		return nil
	}
	langs := make([]string, 0, len(b.store))
	for locale := range b.store {
		langs = append(langs, locale)
	}
	sort.Strings(langs)
	return langs
}

// Keys returns the keys defined in a locale, sorted.
func (b *Bundle) Keys(locale string) []string {
	keys := make([]string, 0, len(b.store[locale]))
	for k := range b.store[locale] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Save persists the active locale so the choice survives across runs.
func (b *Bundle) Save() error {
	if b == nil {
		return nil
	}
	dir := LocaleDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".current"), []byte(b.current+"\n"), 0o600)
}

// LoadCurrent reads the previously saved active locale, if any.
func (b *Bundle) LoadCurrent() string {
	data, err := os.ReadFile(filepath.Join(LocaleDir(), ".current"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ValidLocale reports whether s looks like a BCP-47 locale tag (aa-AA).
func ValidLocale(s string) bool {
	if len(s) < 2 || len(s) > 35 {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

// InvalidLocaleError is returned for malformed locale names or files.
type InvalidLocaleError struct {
	Locale string
	Reason string
}

func (e *InvalidLocaleError) Error() string {
	if e.Reason != "" {
		return "invalid locale " + e.Locale + ": " + e.Reason
	}
	return "invalid locale " + e.Locale
}

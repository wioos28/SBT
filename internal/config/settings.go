// Package config stores SBT runtime preferences at ~/.sbt/config.toml.
//
// The store is deliberately small and safe: values are scalars, keys are
// validated against a declared schema, secret-looking keys are redacted in all
// human-readable output, and a configuration value is never executed as a shell
// command.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Settings holds the key/value preferences of the current user.
type Settings struct {
	values map[string]any
}

// DefaultSettings returns the built-in defaults for every declared key.
func DefaultSettings() *Settings {
	cfg := &Settings{values: map[string]any{}}
	for _, d := range Schema {
		cfg.values[d.Key] = d.Default
	}
	return cfg
}

// configDir returns the SBT state directory (~/.sbt).
func configDir() string {
	if v := os.Getenv("SBT_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".sbt")
	}
	return filepath.Join(home, ".sbt")
}

// Dir is the exported SBT state directory used by the other modules.
func Dir() string { return configDir() }

func defaultConfigPath() string { return filepath.Join(configDir(), "config.toml") }

// Path is the on-disk location of the configuration file.
func Path() string { return defaultConfigPath() }

// Load reads the config file, merging it over the defaults. A missing file is
// not an error: defaults are returned.
func Load() (*Settings, error) {
	cfg := DefaultSettings()
	data, err := os.ReadFile(defaultConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "[") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		cfg.values[key] = parseScalar(strings.TrimSpace(parts[1]))
	}
	return cfg, nil
}

// Save writes the configuration atomically with owner-only permissions.
func (s *Settings) Save() error {
	if s == nil || s.values == nil {
		return nil
	}
	path := defaultConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# SBT user configuration\n")
	b.WriteString("# Secret-looking keys are never printed by `sbt setting get`.\n")
	for _, k := range s.keys() {
		fmt.Fprintf(&b, "%s = %s\n", k, formatScalar(s.values[k]))
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Settings) keys() []string {
	keys := make([]string, 0, len(s.values))
	for k := range s.values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Keys returns every configured key, sorted.
func (s *Settings) Keys() []string {
	if s == nil {
		return nil
	}
	return s.keys()
}

// Set stores one key/value pair, refusing unknown keys so a typo cannot create
// dead configuration.
func (s *Settings) Set(key string, value any) error {
	if s == nil {
		return fmt.Errorf("no settings loaded")
	}
	if _, known := SchemaKey(key); !known {
		return fmt.Errorf("unknown setting %q", key)
	}
	if s.values == nil {
		s.values = map[string]any{}
	}
	s.values[key] = value
	return nil
}

// SetRaw stores a key without schema validation (internal use only).
func (s *Settings) SetRaw(key string, value any) {
	if s == nil {
		return
	}
	if s.values == nil {
		s.values = map[string]any{}
	}
	s.values[key] = value
}

// Get returns the configured value and whether it exists.
func (s *Settings) Get(key string) (any, bool) {
	if s == nil || s.values == nil {
		return nil, false
	}
	v, ok := s.values[key]
	return v, ok
}

// GetAll returns a copy of every value.
func (s *Settings) GetAll() map[string]any {
	if s == nil || s.values == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(s.values))
	for k, v := range s.values {
		out[k] = v
	}
	return out
}

// GetString returns a string value.
func (s *Settings) GetString(key string) (string, bool) {
	v, ok := s.Get(key)
	if !ok {
		return "", false
	}
	out, ok := v.(string)
	return out, ok
}

// GetBool returns a bool value.
func (s *Settings) GetBool(key string) (bool, bool) {
	v, ok := s.Get(key)
	if !ok {
		return false, false
	}
	out, ok := v.(bool)
	return out, ok
}

// GetInt returns an int value.
func (s *Settings) GetInt(key string) (int, bool) {
	v, ok := s.Get(key)
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	default:
		return 0, false
	}
}

// parseScalar decodes a TOML-ish scalar. It never evaluates anything.
func parseScalar(value string) any {
	lower := strings.ToLower(value)
	switch {
	case lower == "true":
		return true
	case lower == "false":
		return false
	case strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2:
		return strings.Trim(value, "\"")
	case strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") && len(value) >= 2:
		return strings.Trim(value, "'")
	default:
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
		return value
	}
}

func formatScalar(v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case string:
		return strconv.Quote(x)
	default:
		return strconv.Quote(fmt.Sprintf("%v", x))
	}
}

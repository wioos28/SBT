package tui

import "github.com/wioos28/sbt/internal/i18n"

// User-facing text.
//
// Every string the interface shows goes through tr. The second argument is the
// English text, and it is what a missing translation falls back to - so a locale
// that does not cover a key shows readable English rather than a raw key, and
// adding a language never means translating the whole interface before it can be
// switched on.
//
// The lookup reads the process-wide bundle, which the session selects at startup
// and the Settings view changes live, so switching language takes effect on the
// next frame without a restart.

func tr(key, fallback string) string {
	if s := i18n.T(key); s != "" && s != key {
		return s
	}
	return fallback
}

// onOff renders a boolean the way the settings table shows it.
func trOnOff(v bool) string {
	if v {
		return tr("common.on", "ON")
	}
	return tr("common.off", "OFF")
}

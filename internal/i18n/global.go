package i18n

import "sync"

// The package-level bundle lets leaf packages call i18n.T("warning.critical")
// without threading a bundle through every call. The application sets it once at
// startup with Use / SetDefault.
var (
	globalMu     sync.RWMutex
	globalBundle = NewBundle()
)

// Default returns the process-wide bundle.
func Default() *Bundle {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalBundle
}

// SetDefault replaces the process-wide bundle (used by the app at startup).
func SetDefault(b *Bundle) {
	if b == nil {
		return
	}
	globalMu.Lock()
	globalBundle = b
	globalMu.Unlock()
}

// T translates a key with the process-wide bundle.
func T(key string) string { return Default().T(key) }

// Tf translates and formats a key with the process-wide bundle.
func Tf(key string, args ...any) string { return Default().Tf(key, args...) }

// Use selects the active language on the process-wide bundle.
func Use(locale string) error { return Default().SetLanguage(locale) }

// Current returns the active language of the process-wide bundle.
func Current() string { return Default().Current() }

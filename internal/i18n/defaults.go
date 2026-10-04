package i18n

// defaultStrings returns the built-in translations for a locale.
func defaultStrings(locale string) map[string]string {
	switch locale {
	case "vi-VN":
		return viVN()
	case "ru-RU":
		return ruRU()
	case "zh-CN":
		return zhCN()
	default:
		return enUS()
	}
}

func enUS() map[string]string {
	return map[string]string{
		KeyLevelInfo: "INFO", KeyLevelNotice: "NOTICE", KeyLevelWarning: "WARNING",
		KeyLevelDanger: "DANGER", KeyLevelCritical: "CRITICAL",

		KeyWarningCritical: "Critical Warning", KeyWarningDanger: "Danger", KeyWarningWarning: "Warning",

		KeyNavDashboard: "Dashboard", KeyNavSandbox: "Sandbox", KeyNavModels: "Models",
		KeyNavAI: "AI", KeyNavAICLI: "AI CLI", KeyNavFiles: "Files",
		KeyNavWorkspace: "Workspace", KeyNavProcesses: "Processes", KeyNavSecurity: "Security",
		KeyNavLogs: "Logs", KeyNavSettings: "Settings",

		KeyCommonOK: "OK", KeyCommonCancel: "Cancel", KeyCommonBack: "Back",
		KeyCommonYes: "Yes", KeyCommonNo: "No", KeyCommonConfirm: "Confirm",
		KeyCommonApply: "Apply Fix", KeyCommonReset: "Reset", KeyCommonNone: "none",
		KeyCommonEnabled: "enabled", KeyCommonDisabled: "disabled", KeyCommonSearch: "Search",
		KeyCommonLoading: "loading", KeyCommonUnknown: "unknown", KeyCommonVerified: "VERIFIED",
		KeyCommonFailed: "FAILED", KeyCommonWarning: "WARNING", KeyCommonRestart: "Restart",
		KeyCommonInspect: "Inspect", KeyCommonDestroy: "Destroy", KeyCommonKeep: "Keep",
		KeyCommonDeleteAll: "Delete All",

		KeyStatusReady: "Ready", KeyStatusRunning: "Running", KeyStatusStopped: "Stopped",

		KeyActionSandboxStart: "start", KeyActionSandboxStop: "stop",
		KeyActionSandboxKill: "destroy", KeyActionPanic: "panic",
		KeyActionExport: "Export Changes", KeyActionDiscard: "Discard Changes",
		KeyActionSnapshot: "Snapshot", KeyActionDestroy: "Destroy",
		KeyConfirmDestructive: "This action cannot be undone.",
		KeyConfirmDestroySbx:  "Destroy this sandbox and everything SBT created for it?",
		KeyConfirmDeleteSnaps: "This sandbox has %d snapshots. Delete them too?",
		KeyConfirmPanic:       "Enter panic mode? Running processes will be frozen, not destroyed.",
		KeyConfirmHighRisk:    "Relaxing the sandbox policy lowers its guarantees. Continue?",
		KeyDangerExplicit:     "Type the sandbox name to confirm.",
		KeyCriticalIsolation:  "SANDBOX ISOLATION FAILED",
		KeyLANAuthRequired:    "Authentication required",
		KeySettingsSaved:      "settings saved",
		KeySettingUnknown:     "unknown setting",
	}
}

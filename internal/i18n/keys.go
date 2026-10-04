package i18n

import "fmt"

// sprintf is a tiny wrapper so bundle code keeps formatting identical to
// fmt.Sprintf without importing fmt at every call site.
func sprintf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// Key catalog. Every user-facing string used by the TUI, CLI and web UI is a
// constant here so a missing translation is obvious.
const (
	KeyLevelInfo     = "level.info"
	KeyLevelNotice   = "level.notice"
	KeyLevelWarning  = "level.warning"
	KeyLevelDanger   = "level.danger"
	KeyLevelCritical = "level.critical"

	KeyWarningCritical = "warning.critical"
	KeyWarningDanger   = "warning.danger"
	KeyWarningWarning  = "warning.warning"

	KeyNavDashboard = "nav.dashboard"
	KeyNavSandbox   = "nav.sandbox"
	KeyNavModels    = "nav.models"
	KeyNavAI        = "nav.ai"
	KeyNavAICLI     = "nav.aicli"
	KeyNavFiles     = "nav.files"
	KeyNavWorkspace = "nav.workspace"
	KeyNavProcesses = "nav.processes"
	KeyNavSecurity  = "nav.security"
	KeyNavLogs      = "nav.logs"
	KeyNavSettings  = "nav.settings"

	KeyCommonOK        = "common.ok"
	KeyCommonCancel    = "common.cancel"
	KeyCommonBack      = "common.back"
	KeyCommonYes       = "common.yes"
	KeyCommonNo        = "common.no"
	KeyCommonConfirm   = "common.confirm"
	KeyCommonApply     = "common.apply"
	KeyCommonReset     = "common.reset"
	KeyCommonNone      = "common.none"
	KeyCommonEnabled   = "common.enabled"
	KeyCommonDisabled  = "common.disabled"
	KeyCommonSearch    = "common.search"
	KeyCommonLoading   = "common.loading"
	KeyCommonUnknown   = "common.unknown"
	KeyCommonVerified  = "common.verified"
	KeyCommonFailed    = "common.failed"
	KeyCommonWarning   = "common.warning"
	KeyCommonRestart   = "common.restart"
	KeyCommonInspect   = "common.inspect"
	KeyCommonDestroy   = "common.destroy"
	KeyCommonKeep      = "common.keep"
	KeyCommonDeleteAll = "common.delete_all"

	KeyStatusReady   = "status.ready"
	KeyStatusRunning = "status.running"
	KeyStatusStopped = "status.stopped"

	KeyActionSandboxStart = "action.sandbox_start"
	KeyActionSandboxStop  = "action.sandbox_stop"
	KeyActionSandboxKill  = "action.sandbox_kill"
	KeyActionPanic        = "action.panic"
	KeyActionExport       = "action.export"
	KeyActionDiscard      = "action.discard"
	KeyActionSnapshot     = "action.snapshot"
	KeyActionDestroy      = "action.destroy"

	KeyConfirmDestructive = "confirm.destructive"
	KeyConfirmDestroySbx  = "confirm.destroy_sandbox"
	KeyConfirmDeleteSnaps = "confirm.delete_snapshots"
	KeyConfirmPanic       = "confirm.panic"
	KeyConfirmHighRisk    = "confirm.high_risk"
	KeyDangerExplicit     = "danger.explicit_confirmation"
	KeyCriticalIsolation  = "critical.isolation_failed"
	KeyLANAuthRequired    = "lan.auth_required"
	KeySettingsSaved      = "settings.saved"
	KeySettingUnknown     = "setting.unknown"
)

package i18n

func zhCN() map[string]string {
	return map[string]string{
		KeyLevelInfo: "信息", KeyLevelNotice: "注意", KeyLevelWarning: "警告",
		KeyLevelDanger: "危险", KeyLevelCritical: "严重",

		KeyWarningCritical: "严重警告", KeyWarningDanger: "危险", KeyWarningWarning: "警告",

		KeyNavDashboard: "仪表盘", KeyNavSandbox: "沙箱", KeyNavModels: "模型",
		KeyNavAI: "人工智能", KeyNavAICLI: "AI 命令行", KeyNavFiles: "文件",
		KeyNavWorkspace: "工作区", KeyNavProcesses: "进程", KeyNavSecurity: "安全",
		KeyNavLogs: "日志", KeyNavSettings: "设置",

		KeyCommonOK: "确定", KeyCommonCancel: "取消", KeyCommonBack: "返回",
		KeyCommonYes: "是", KeyCommonNo: "否", KeyCommonConfirm: "确认",
		KeyCommonApply: "应用修复", KeyCommonReset: "重置", KeyCommonNone: "无",
		KeyCommonEnabled: "已启用", KeyCommonDisabled: "已禁用", KeyCommonSearch: "搜索",
		KeyCommonLoading: "加载中", KeyCommonUnknown: "未知", KeyCommonVerified: "已验证",
		KeyCommonFailed: "失败", KeyCommonWarning: "警告", KeyCommonRestart: "重启",
		KeyCommonInspect: "检查", KeyCommonDestroy: "销毁", KeyCommonKeep: "保留",
		KeyCommonDeleteAll: "全部删除",

		KeyStatusReady: "就绪", KeyStatusRunning: "运行中", KeyStatusStopped: "已停止",

		KeyActionSandboxStart: "启动", KeyActionSandboxStop: "停止",
		KeyActionSandboxKill: "销毁", KeyActionPanic: "紧急",
		KeyActionExport: "导出更改", KeyActionDiscard: "丢弃更改",
		KeyActionSnapshot: "快照", KeyActionDestroy: "销毁",
		KeyConfirmDestructive: "此操作无法撤销。",
		KeyConfirmDestroySbx:  "销毁此沙箱及其全部 SBT 资源？",
		KeyConfirmDeleteSnaps: "此沙箱有 %d 个快照。一并删除吗？",
		KeyConfirmPanic:       "进入紧急模式？进程将被冻结，而不会被销毁。",
		KeyConfirmHighRisk:    "放宽沙箱策略会降低保障。是否继续？",
		KeyDangerExplicit:     "输入沙箱名称以确认。",
		KeyCriticalIsolation:  "沙箱隔离失败",
		KeyLANAuthRequired:    "需要身份验证",
		KeySettingsSaved:      "设置已保存",
		KeySettingUnknown:     "未知设置",
	}
}

package i18n

func uiZH() map[string]string {
	return map[string]string{
		"common.on":  "开",
		"common.off": "关",

		"settings.section.appearance": "外观",
		"settings.section.colors":     "颜色",
		"settings.section.animations": "动画",
		"settings.section.terminal":   "终端",
		"settings.section.security":   "安全",
		"settings.section.language":   "语言",
		"settings.section.troll":      "趣味",
		"settings.section.advanced":   "高级",

		"settings.hint.nav": "左右切换分区  上下切换行  enter 编辑  esc 返回",
		"settings.saved":    "已保存",
		"settings.notSaved": "未保存",
		"settings.open":     "enter",

		"permission.required":   "需要权限",
		"permission.capability": "权限",
		"permission.reason":     "原因",
		"permission.risk":       "风险",
		"permission.evidence":   "依据",
		"permission.allowOnce":  "[ 允许一次 ]",
		"permission.allowSess":  "[ 本次会话允许 ]",
		"permission.configure":  "[ 配置 ]",
		"permission.cancel":     "[ 取消 ]",
		"permission.esc":        "esc = 取消",

		"destroy.title":    "严重安全事件",
		"destroy.typeLine": "输入  DESTROY SBT SANDBOX  以解锁",
		"destroy.locked":   "已锁定  [ 请输入完全一致的短语 ]",
		"destroy.unlocked": "已解锁  [ enter 销毁沙箱 ]",
		"destroy.keep":     "保留沙箱",

		"welcome.subtitle": "安全终端",
		"welcome.enter":    "按任意键进入终端",
		"welcome.commands": "/help   /setting   /permissions   /status",

		"boot.anyKey":   "按任意键跳过",
		"boot.starting": "SBT 启动中",

		"perms.needs": "需要:",
		"perms.least": "最小权限:",
		"perms.title": "权限",
		"perms.none":  "尚无权限报告",

		"files.none": "没有文件",
		"files.hint": "左右选择  enter 打开",

		"slash.unknown": "未知命令",
		"slash.help":    "试试 /help",

		"alert.isolationLimited": "隔离受限",
		"alert.cageBroken":       "隔离笼损坏",
		"alert.partial":          "部分隔离机制未能验证",
		"alert.refused":          "在验证隔离之前拒绝危险执行",
		"repair.done":            "隔离笼已重新初始化",
		"repair.stillBroken":     "隔离笼仍然损坏",
	}
}

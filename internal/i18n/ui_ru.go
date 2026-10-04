package i18n

func uiRU() map[string]string {
	return map[string]string{
		"common.on":  "ВКЛ",
		"common.off": "ВЫКЛ",

		"settings.section.appearance": "Оформление",
		"settings.section.colors":     "Цвета",
		"settings.section.animations": "Анимация",
		"settings.section.terminal":   "Терминал",
		"settings.section.security":   "Безопасность",
		"settings.section.language":   "Язык",
		"settings.section.troll":      "Развлечения",
		"settings.section.advanced":   "Дополнительно",

		"settings.hint.nav": "влево/вправо раздел  вверх/вниз строка  enter изменить  esc назад",
		"settings.saved":    "сохранено",
		"settings.notSaved": "не сохранено",
		"settings.open":     "enter",

		"permission.required":   "ТРЕБУЕТСЯ РАЗРЕШЕНИЕ",
		"permission.capability": "Возможность",
		"permission.reason":     "Причина",
		"permission.risk":       "Риск",
		"permission.evidence":   "Основание",
		"permission.allowOnce":  "[ Разрешить один раз ]",
		"permission.allowSess":  "[ Разрешить до конца сессии ]",
		"permission.configure":  "[ Настроить ]",
		"permission.cancel":     "[ Отмена ]",
		"permission.esc":        "esc = отмена",

		"destroy.title":    "КРИТИЧЕСКОЕ СОБЫТИЕ БЕЗОПАСНОСТИ",
		"destroy.typeLine": "введите  DESTROY SBT SANDBOX  чтобы разблокировать",
		"destroy.locked":   "заблокировано  [ введите точную фразу ]",
		"destroy.unlocked": "РАЗБЛОКИРОВАНО  [ enter уничтожит песочницу ]",
		"destroy.keep":     "сохранить песочницу",

		"welcome.subtitle": "Безопасный терминал",
		"welcome.enter":    "нажмите любую клавишу, чтобы войти",
		"welcome.commands": "/help   /setting   /permissions   /status",

		"boot.anyKey":   "любая клавиша пропускает",
		"boot.starting": "SBT запускается",

		"perms.needs": "требуется:",
		"perms.least": "минимальные права:",
		"perms.title": "права",
		"perms.none":  "отчёта о правах ещё нет",

		"files.none": "нет файлов",
		"files.hint": "влево/вправо  enter открыть",

		"slash.unknown": "нет такой команды",
		"slash.help":    "попробуйте /help",

		"alert.isolationLimited": "ИЗОЛЯЦИЯ ОГРАНИЧЕНА",
		"alert.cageBroken":       "ПЕСОЧНИЦА СЛОМАНА",
		"alert.partial":          "часть механизмов изоляции не подтверждена",
		"alert.refused":          "опасные операции отклонены до подтверждения изоляции",
		"repair.done":            "песочница переинициализирована",
		"repair.stillBroken":     "песочница всё ещё сломана",
	}
}

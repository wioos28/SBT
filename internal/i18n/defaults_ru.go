package i18n

func ruRU() map[string]string {
	return map[string]string{
		KeyLevelInfo: "ИНФО", KeyLevelNotice: "ВНИМАНИЕ", KeyLevelWarning: "ПРЕДУПРЕЖДЕНИЕ",
		KeyLevelDanger: "ОПАСНОСТЬ", KeyLevelCritical: "КРИТИЧНО",

		KeyWarningCritical: "Критическое предупреждение", KeyWarningDanger: "Опасность", KeyWarningWarning: "Предупреждение",

		KeyNavDashboard: "Панель", KeyNavSandbox: "Песочница", KeyNavModels: "Модели",
		KeyNavAI: "ИИ", KeyNavAICLI: "ИИ CLI", KeyNavFiles: "Файлы",
		KeyNavWorkspace: "Рабочая область", KeyNavProcesses: "Процессы", KeyNavSecurity: "Безопасность",
		KeyNavLogs: "Журналы", KeyNavSettings: "Настройки",

		KeyCommonOK: "ОК", KeyCommonCancel: "Отмена", KeyCommonBack: "Назад",
		KeyCommonYes: "Да", KeyCommonNo: "Нет", KeyCommonConfirm: "Подтвердить",
		KeyCommonApply: "Применить", KeyCommonReset: "Сброс", KeyCommonNone: "нет",
		KeyCommonEnabled: "вкл", KeyCommonDisabled: "выкл", KeyCommonSearch: "Поиск",
		KeyCommonLoading: "загрузка", KeyCommonUnknown: "неизвестно", KeyCommonVerified: "ПРОВЕРЕНО",
		KeyCommonFailed: "ОШИБКА", KeyCommonWarning: "ПРЕДУПРЕЖДЕНИЕ", KeyCommonRestart: "Перезапуск",
		KeyCommonInspect: "Проверить", KeyCommonDestroy: "Уничтожить", KeyCommonKeep: "Оставить",
		KeyCommonDeleteAll: "Удалить все",

		KeyStatusReady: "Готово", KeyStatusRunning: "Работает", KeyStatusStopped: "Остановлено",

		KeyActionSandboxStart: "запуск", KeyActionSandboxStop: "стоп",
		KeyActionSandboxKill: "уничтожить", KeyActionPanic: "паника",
		KeyActionExport: "Экспорт изменений", KeyActionDiscard: "Отменить изменения",
		KeyActionSnapshot: "Снимок", KeyActionDestroy: "Уничтожить",
		KeyConfirmDestructive: "Это действие необратимо.",
		KeyConfirmDestroySbx:  "Уничтожить эту песочницу и всё, что создал SBT?",
		KeyConfirmDeleteSnaps: "У этой песочницы %d снимков. Удалить их тоже?",
		KeyConfirmPanic:       "Войти в режим паники? Процессы будут заморожены, а не уничтожены.",
		KeyConfirmHighRisk:    "Ослабление политики снижает гарантии. Продолжить?",
		KeyDangerExplicit:     "Введите имя песочницы для подтверждения.",
		KeyCriticalIsolation:  "ИЗОЛЯЦИЯ ПЕСОЧНИЦЫ НАРУШЕНА",
		KeyLANAuthRequired:    "Требуется аутентификация",
		KeySettingsSaved:      "настройки сохранены",
		KeySettingUnknown:     "неизвестная настройка",
	}
}

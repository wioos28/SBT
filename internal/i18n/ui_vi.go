package i18n

func uiVI() map[string]string {
	return map[string]string{
		"common.on":  "BẬT",
		"common.off": "TẮT",

		"settings.section.appearance": "Giao diện",
		"settings.section.colors":     "Màu sắc",
		"settings.section.animations": "Hiệu ứng",
		"settings.section.terminal":   "Terminal",
		"settings.section.security":   "Bảo mật",
		"settings.section.language":   "Ngôn ngữ",
		"settings.section.troll":      "Vui vẻ",
		"settings.section.advanced":   "Nâng cao",

		"settings.hint.nav": "trái/phải đổi mục  lên/xuống đổi dòng  enter sửa  esc quay lại",
		"settings.saved":    "đã lưu",
		"settings.notSaved": "chưa lưu",
		"settings.open":     "enter",

		"permission.required":   "CẦN QUYỀN",
		"permission.capability": "Quyền",
		"permission.reason":     "Lý do",
		"permission.risk":       "Rủi ro",
		"permission.evidence":   "Bằng chứng",
		"permission.allowOnce":  "[ Cho phép một lần ]",
		"permission.allowSess":  "[ Cho phép cả phiên ]",
		"permission.configure":  "[ Cấu hình ]",
		"permission.cancel":     "[ Huỷ ]",
		"permission.esc":        "esc = huỷ",

		"destroy.title":    "SỰ KIỆN BẢO MẬT NGHIÊM TRỌNG",
		"destroy.typeLine": "gõ  DESTROY SBT SANDBOX  để mở khoá",
		"destroy.locked":   "đang khoá  [ gõ đúng cụm từ ]",
		"destroy.unlocked": "ĐÃ MỞ KHOÁ  [ enter để huỷ sandbox ]",
		"destroy.keep":     "giữ sandbox",

		"welcome.subtitle": "Terminal An toàn",
		"welcome.enter":    "nhấn phím bất kỳ để vào terminal",
		"welcome.commands": "/help   /setting   /permissions   /status",

		"boot.anyKey":   "nhấn phím bất kỳ để bỏ qua",
		"boot.starting": "SBT đang khởi động",

		"perms.needs": "cần:",
		"perms.least": "đặc quyền tối thiểu:",
		"perms.title": "quyền",
		"perms.none":  "chưa có báo cáo quyền",

		"files.none": "không có tệp nào",
		"files.hint": "trái/phải  enter để mở",

		"slash.unknown": "không có lệnh",
		"slash.help":    "thử /help",

		"alert.isolationLimited": "CÔ LẬP GIỚI HẠN",
		"alert.cageBroken":       "LỒNG CHUỒNG HỎNG",
		"alert.partial":          "một số cơ chế cô lập không được xác minh",
		"alert.refused":          "không chạy tác vụ nguy hiểm cho tới khi xác minh được cô lập",
		"repair.done":            "đã khởi tạo lại lồng chuồng",
		"repair.stillBroken":     "lồng chuồng vẫn hỏng",
	}
}

package i18n

func viVN() map[string]string {
	return map[string]string{
		KeyLevelInfo: "THÔNG TIN", KeyLevelNotice: "LƯU Ý", KeyLevelWarning: "CẢNH BÁO",
		KeyLevelDanger: "NGUY HIỂM", KeyLevelCritical: "NGHIÊM TRỌNG",

		KeyWarningCritical: "Cảnh báo nghiêm trọng", KeyWarningDanger: "Nguy hiểm", KeyWarningWarning: "Cảnh báo",

		KeyNavDashboard: "Bảng điều khiển", KeyNavSandbox: "Hộp cát", KeyNavModels: "Mô hình",
		KeyNavAI: "AI", KeyNavAICLI: "AI CLI", KeyNavFiles: "Tệp",
		KeyNavWorkspace: "Không gian làm việc", KeyNavProcesses: "Tiến trình", KeyNavSecurity: "Bảo mật",
		KeyNavLogs: "Nhật ký", KeyNavSettings: "Cài đặt",

		KeyCommonOK: "Đồng ý", KeyCommonCancel: "Hủy", KeyCommonBack: "Quay lại",
		KeyCommonYes: "Có", KeyCommonNo: "Không", KeyCommonConfirm: "Xác nhận",
		KeyCommonApply: "Áp dụng sửa", KeyCommonReset: "Đặt lại", KeyCommonNone: "không",
		KeyCommonEnabled: "bật", KeyCommonDisabled: "tắt", KeyCommonSearch: "Tìm kiếm",
		KeyCommonLoading: "đang tải", KeyCommonUnknown: "không rõ", KeyCommonVerified: "ĐÃ XÁC MINH",
		KeyCommonFailed: "THẤT BẠI", KeyCommonWarning: "CẢNH BÁO", KeyCommonRestart: "Khởi động lại",
		KeyCommonInspect: "Kiểm tra", KeyCommonDestroy: "Hủy", KeyCommonKeep: "Giữ",
		KeyCommonDeleteAll: "Xóa tất cả",

		KeyStatusReady: "Sẵn sàng", KeyStatusRunning: "Đang chạy", KeyStatusStopped: "Đã dừng",

		KeyActionSandboxStart: "khởi động", KeyActionSandboxStop: "dừng",
		KeyActionSandboxKill: "hủy", KeyActionPanic: "hoảng loạn",
		KeyActionExport: "Xuất thay đổi", KeyActionDiscard: "Bỏ thay đổi",
		KeyActionSnapshot: "Ảnh chụp", KeyActionDestroy: "Hủy",
		KeyConfirmDestructive: "Hành động này không thể hoàn tác.",
		KeyConfirmDestroySbx:  "Hủy hộp cát này và mọi thứ SBT đã tạo cho nó?",
		KeyConfirmDeleteSnaps: "Hộp cát này có %d ảnh chụp. Xóa luôn chứ?",
		KeyConfirmPanic:       "Vào chế độ hoảng loạn? Tiến trình sẽ bị đóng băng, không bị hủy.",
		KeyConfirmHighRisk:    "Nới lỏng chính sách hộp cát làm giảm bảo đảm. Tiếp tục?",
		KeyDangerExplicit:     "Nhập tên hộp cát để xác nhận.",
		KeyCriticalIsolation:  "CÔ LẬP HỘP CÁT THẤT BẠI",
		KeyLANAuthRequired:    "Yêu cầu xác thực",
		KeySettingsSaved:      "đã lưu cài đặt",
		KeySettingUnknown:     "cài đặt không rõ",
	}
}

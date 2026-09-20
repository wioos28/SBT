# TODO — Hoàn thiện TUI "cage" (handoff)

> Trạng thái: build `./internal/tui` đang **LỖI** (2 file bị chèn dở), phần còn lại
> của engine (theme, glyphs, buffer, screen, input, reader, poll, draw) đã viết xong.
> Backend (journal / monitor / jail workspace / shared) đã xong và đã pass test.

## 0) Việc cấp bách: sửa 2 chỗ chèn bị cắt cụt (build hỏng tại đây)

### `internal/tui/input.go`
- Dòng ~150-153: `parseKey` bị cắt giữa switch. Sau `case b == '\t':` còn thiếu:

```go
	return Key{Type: KeyTab}, 1
case b == 127:
	return Key{Type: KeyBackspace}, 1
case b == 0:
	return Key{Type: KeyRune, Rune: ' ', Ctrl: true}, 1
case b < 27:
	return Key{Type: KeyRune, Rune: rune('a' + b - 1), Ctrl: true}, 1
case b < 32:
	return Key{}, 1
default:
	r, size := utf8.DecodeRune(seq)
	if r == utf8.RuneError && size <= 1 {
		if len(seq) < utf8.UTFMax {
			return Key{}, 0
		}
		return Key{}, 1
	}
	return Key{Type: KeyRune, Rune: r, Shift: isUpper(r)}, size
}
}
```

(đóng hàm `parseKey` trước khi `parseCSI`, `csiFinal`, `parseTilde`, `parseSS3`,
`modifiers`, `modifierFlags`, `splitParams`, `atoiOK`, `isUpper` bắt đầu).

### `internal/tui/render.go`
- Dòng ~485: hàm `changesView` bị cắt trước khi đóng. Sau
  `start := st.clampScroll(len(rows), inner.H)` còn thiếu vòng vẽ + đóng hàm:

```go
	for j := 0; j < inner.H && start+j < len(rows); j++ {
		rw := rows[start+j]
		sty := Style{Fg: p.Text}
		switch rw.state {
		case StateOK:
			sty.Fg = p.Green
		case StateWarn:
			sty.Fg = p.Yellow
		case StateDanger:
			sty.Fg = p.Red
		case StateMeta:
			sty.Fg = p.Muted
		}
		b.WriteClipped(inner.X, inner.Y+j, inner.Right(), rw.text, sty)
	}
}
```

## 1) Kiểu/hàm được tham chiếu nhưng CHƯA tồn tại (phải bổ sung)

Trong `internal/tui/model.go` / `ui_state.go` đang dùng các tên dưới đây — thêm
định nghĩa (khuyên để trong `model.go`):

- `type FileInfo struct { Path string; Size string; Executable bool; Kind workspace.Kind }`
  (đang được `Snapshot.Files []FileInfo`, `NewExportSel`, `executableSelection` dùng).
- `type PolicyChoice int` + hằng `PolicyLow / PolicyHigh` (dùng trong `Confirm.Policy`,
  `askPolicy`, event `evSetPolicy`) — hoặc đổi sang `policy.Preset` sẵn có.
- `type CageState int` + `CageSolid / CageLimited / CageBroken` + struct
  `Cage struct { State CageState; Reason string }` (hiện `Snapshot` khai
  `Cage CageState` — render đã dùng `s.Cage.State` nên chọn dạng struct).
- `type BootState struct { Started time.Time; Done bool }` (dùng trong `UIState.Boot`).
- `modeWord(m policy.Mode) string`, `policyWord(m policy.Mode, p policy.Preset) string`.
- `fields(s string) []string` (có thể dùng `strings.Fields`).
- `osGetenv(k string) string` — wrapper mỏng cho `os.Getenv` để test.
- `min(a,b int) int` — Go 1.21+ có builtin (module là go1.25): đừng định nghĩa lại;
  `max` trong ui_state.go cũng vậy nếu compiler chấp nhận builtin.
- Xóa hàm rác `promptDropRun`, `itoa64`, `stateColor/dangerColor/...` nếu compiler
  báo unused/undefined (chúng nằm rải rác trong render.go/ui_state.go).
- `HelpLines` (biến `[]string` nội dung view help) — chưa có.
- `Palette` (registry lệnh) + `func (p *Palette) entries() []PaletteEntry` +
  trường `i.Palette` trên `Interpreter` — overlay.go đang gọi; đơn giản nhất:
  thêm `Palette *Palette` vào struct `Interpreter` và một registry mặc định
  `defaultPalette()` với các entry: switch view 1-5, run input, export all,
  discard (mở confirm), exit (mở confirm), help.
- `Interpreter.Palette` nil-safe trong `drawPalette` (nếu nil → bỏ qua vẽ).

## 2) Lỗi logic cần vá ngay khi build xanh

- `internal/tui/overlay.go` — `drawConfirm`: soát dấu ngoặc đoạn
  `choice := "run it"` tới hết hàm (một lần chèn bị lệch).
- `internal/tui/render.go` — `computeLayout`: `l.Side.W*l.SideBool()` không tồn tại
  (`Side` là `Rect`, không có `SideBool`). Sửa: `sideW := 0; if ... { sideW = 36;
  l.Side = ... }` rồi `l.Work.W = w - col - sideW`.
- `internal/tui/render.go` — `resourcesPanel` dùng `time.Second` nhưng file chưa
  import `time`; kiểm tra `itoa/formatBytes/formatDuration/clockTime/pad2` tồn tại
  đúng 1 lần (một khối helpers từng bị edit thay mất).
- `internal/tui/ui_state.go` — kiểm tra không còn mảnh code trùng (đã có 1 lần
  dính khối `clampScroll` lặp); `gofmt` + build sẽ lộ ngay.
- `internal/tui/controls.go` — `askExit` dùng `snap.Counts()`, `c.Summary()`,
  `c.Empty()` — đảm bảo `workspace.Counts` có đúng tên method này.
- `UIState.handleKey` trả `event{kind: evStop}` cho hầu hết phím — `App` phải
  hiểu `evStop` = "đã xử lý, chỉ cần vẽ lại", KHÔNG phải dừng session.

## 3) Chưa viết: lớp App + kích hoạt từ session

- File `internal/tui/app.go`:
  - struct `App { Theme *Theme; Interp *Interpreter; Screen *Screen; Reader *KeyReader;
    State *UIState; Snap Snapshot; Out chan<- Request; Palette *Palette }`.
  - `func (a *App) Run(ctx, onFrame func(*Snapshot)) error`: vòng lặp
    read-key → `handleKey` → map event → đẩy `Request` ra kênh session:
    - `evRun` → `Request{Kind: ReqRun, Argv}` (session sẽ Suspend screen, chạy
      jail, Resume, rồi cập nhật Snapshot).
    - `evSetPolicy` → `Request{ReqSetPolicy, Policy: preset}`.
    - `evExport` → `Request{ReqExport, Paths, Destination, Overwrite}`.
    - `evDiscard` → `Request{ReqDiscard}`; `evExit` → `Request{ReqExit}`.
    - `evStop`/`evNone` → chỉ vẽ lại.
  - Render mỗi event + mỗi tick 500ms khi sandbox đang chạy (để meter sống).
- File `internal/tui/session.go` (hoặc sửa `cmd/sbt`): dựng `Snapshot` từ nguồn
  thật: `monitor.Snapshot` → `Snapshot.Stats`, `journal.Files` → `Snapshot.Files`,
  journal runs → `Snapshot.Runs`, probe isolation → `Snapshot.Isolation`,
  `jailspec` → `Policy`. KHÔNG cho TUI tự đọc /proc hay gọi jail.
- Thoát: `ctrl+d` lần 1 mở confirm; confirm mới gửi `ReqExit`. Sau exit, session
  in summary (clean vs changes) ra stdout thường.

## 4) Test cần bổ sung (đích đến: `go test ./internal/tui` xanh)

- `input_test.go`: bảng case byte→Key (arrow CSI, SS3, Alt+1=ESC'1', Ctrl+K=0x0B,
  tab/enter/backspace, UTF-8 đa byte, CSI cắt ngưỡng trả consume=0).
- `buffer_test.go`: rune rộng 2 ô không tràn, `WriteRight` đè, `Pad/Truncate` giữ độ rộng.
- `render_test.go`: render 120x30 có top bar/rail/2 panel phải/status bar; 60x20
  không còn panel phải; 20x5 hiện thông báo quá nhỏ; overlay palette/confirm vẽ đè.
- `controls_test.go`: alt+1..5 đổi view; esc từ view về terminal; export toggle
  'a'/space; confirm esc chọn safe; enter xác nhận phát đúng event; flash 6s hết
  hạn qua `FlashExpire`.

## 5) Quy trình chạy lại sau mỗi đợt sửa

```sh
gofmt -l internal/tui && go vet ./internal/tui && go build ./... && go test ./...
```

## 6) Đã xong (để khỏi làm lại)

- theme.go, glyphs.go, buffer.go, screen.go, input.go (phần đầu), reader.go,
  poll_linux.go, poll_darwin.go, poll_other.go, draw.go, model.go (wire),
  ui_state.go, controls.go, overlay.go (palette+confirm), render.go (phần lớn
  view; status bar & side panel nằm trong render.go, không có render_status.go).
- Quy ước đã chốt: không dep ngoài stdlib; NO_COLOR/TERM=dumb → plain; box glyph
  hạ ASCII; SBT_NO_MOTION/NO_MOTION; mọi trạng thái luôn kèm chữ (không chỉ màu);
  UI không bao giờ tự chạy sandbox — chỉ gửi `Request` qua kênh cho session.


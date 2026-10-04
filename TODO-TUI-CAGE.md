# TODO — Hoàn thiện TUI "cage" (handoff)

> **Trạng thái: đã hoàn tất toàn bộ.** `gofmt -l internal`, `go vet ./...`,
> `go build ./...` và `go test ./...` đều xanh; `internal/tui` có 43 test.
> Cả ba mục tồn đọng ở mục 7 cũng đã xử lý, cộng thêm menu và một loạt lỗi
> phát hiện khi đọc lại code (xem mục 8).

## 0) Cấp bách: hai chỗ chèn bị cắt (build hỏng) — **ĐÃ XỬ LÝ**

Cả hai chỗ chèn mất (`parseKey` trong `input.go`, vòng vẽ của `changesView`
trong `render.go`) đã được khôi phục trong working tree.

## 1) Kiểu/hàm được tham chiếu nhưng chưa tồn tại — **ĐÃ XỬ LÝ**

`FileInfo`, `PolicyChoice`, `CageState`/`Cage`, `BootState`, `modeWord`,
`policyWord`, `fields`, `osGetenv`, `HelpLines` đã có trong `model.go`.

## 2) Lỗi cú pháp / naming — **ĐÃ XỬ LÝ**

`computeLayout`, `resourcesPanel`, `askExit`, `clampScroll` trùng, và các
helper `format*` đã dọn. `gofmt -l internal` không còn output.

## 3) Lớp App + kích hoạt từ session — **ĐÃ XỬ LÝ**

- `internal/tui/app.go`: struct `App` đầy đủ; vòng lặp đọc phím →
  `handleKey` → `Request`. Một goroutine đọc phím duy nhất cho suốt vòng đời
  (không rò goroutine khi timeout); nhịp vẽ 100ms khi có animation, 1s khi
  đứng yên nên terminal gần như không bị ghi khi không có gì để vẽ.
- `internal/tui/session.go`: `Session` dựng `Snapshot` từ nguồn thật
  (`platform.Detect`, `journal`, `monitor`) và thực thi `Request` qua interface
  `Runner`. UI **không bao giờ** tự chạy sandbox.
- `internal/tui/commands.go`: registry lệnh thật (`CommandSet`/`Command`),
  thay cho cách gắn cứng registry vào *màu* `Palette`.

## 4) Test — **ĐÃ XỬ LÝ**

`internal/tui`: 43 test phủ cage verdict, flash, toast, confirm, palette,
meter easing, render thuần theo thời điểm, menu, diff viewer, quét mọi kích
thước terminal × mọi view để chắc chắn không panic.

## 5) Quy trình chạy lại sau mỗi đợt sửa

```sh
gofmt -l internal && go vet ./... && go build ./... && go test ./...
```

## 6) Đã xong (để khỏi làm lại)

- theme.go, glyphs.go, buffer.go, screen.go, input.go, reader.go, poll_*.go,
  draw.go, model.go (wire), ui_state.go, controls.go, overlay.go, render.go.
- anim.go (engine animation — hàm thuần của đồng hồ do caller cấp), toast.go
  (hệ cảnh báo), app.go, session.go, commands.go.
- Quy ước đã chốt: không dep ngoài stdlib; NO_COLOR/TERM=dumb → plain; box glyph
  hạ ASCII; SBT_NO_MOTION/NO_MOTION; **mọi trạng thái luôn kèm chữ** (không
  chỉ màu); UI không bao giờ tự chạy sandbox — chỉ gửi `Request`.

## 7) Việc còn lại của đợt trước — **ĐÃ XỬ LÝ**

- **`Runner` thật + `RunSession`** — `internal/shell/runner.go` hiện thực đầy
  đủ `tui.Runner`: kiểm tra probe, dựng jail spec (có `WorkspaceIn`/
  `WorkspaceOut` nối vào session journal), spawn helper, chờ, reap, ghi
  manifest, xuất, discard. Nó còn hiện thực `StateReader`, `DiffReader`,
  `StateSetter`, `PolicySetter`.
  `internal/shell/session.go` (`RunSession`) là nơi duy nhất nối App + Session +
  Runner + journal + monitor; `main.go` gọi nó khi `sbt` chạy trong terminal.
  `sbt shell` giữ lại REPL cũ làm fallback.
- **Diff viewer** — `changesView` giờ là hai pane: danh sách thay đổi bên trái,
  nội dung before/after bên phải. `BuildChangeRows`/`Selectable` dùng chung cho
  cả renderer lẫn key handler nên hàng được tô sáng luôn là hàng con trỏ chỉ
  tới. `DiffState.Matches` chặn việc hiện diff của file khác cạnh tên file.
- **`docs/LIMITATIONS.md`** và **`docs/SECURITY.md`** — đã viết, và giải thích
  chính xác vì sao overlayfs bị probe nhưng không dùng.

## 8) Menu tích hợp + lỗi phát hiện khi rà lại code

**Menu** (`internal/tui/commands.go`, `ui_state.go`, `render.go`, `controls.go`):

- Menu bar 5 mục: Session · View · Security · Workspace · Help, mở bằng
  `F10` hoặc `alt+m`. Dropdown neo dưới đúng tiêu đề, tự dịch trái thay vì
  cắt chữ, và **không che verdict của cage** — thanh trạng thái là thứ duy
  nhất không được bị menu che mất.
- Menu item là **data** (`Action`), đi qua đúng `runAction` mà palette dùng, nên
  "menu > View > Changes" và "ctrl+k changes" không thể lệch nhau.
- Mọi hàng nguy hiểm vẫn phải qua confirm; `esc` luôn về phía an toàn.
- High risk mode giờ chạm được (trước đó `ConfirmPolicy`/`PolicyChoice` là code
  chết) và confirm liệt kê *knob cụ thể* thay vì chỉ nhãn.

**Lỗi đã sửa:**

| Lỗi | Vì sao quan trọng |
| --- | --- |
| `evStop` bị dùng làm giá trị "không làm gì" | Không thể bind "dừng sandbox"; tách thành `evNone` + `evStop` thật, thêm `ctrl+.` |
| `needsFrames()` luôn `true` khi bật motion | Repaint 10 lần/giây khi giao diện đứng yên — hao CPU/pin |
| `SetVersionString` có comment nhưng **không có hàm** | Phiên bản hiện trên thanh trên cùng luôn rỗng |
| `Snapshot.Sandbox` không ai điền | "SANDBOX ACTIVE" không bao giờ hiện; monitor luôn "idle" |
| `HelpLines` ghi `alt+1..5` | Có 6 view — phím cuối không được document |
| `killSandboxProcess` chỉ nhận 1 kiểu handle | Chặn runner mới dùng chung đường teardown |
| Diff pane mở cả khi không có gì để xem | Che mất thông báo "no changes recorded" |

## 9) Quy ước đã chốt (nhắc lại cho người sau)

- UI **hỏi**, session **quyết**, runner **làm**. Không handler phím nào chạm
  được kernel.
- Mọi trạng thái đều có **chữ**, không chỉ màu. Màu chỉ là tín hiệu phụ.
- Load thất bại phải **nói ra**. Pane trống lặng lẽ đọc thành "file không đổi",
  đó là một tuyên bố sai.

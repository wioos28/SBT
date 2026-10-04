# TODO — Hoàn thiện TUI "cage" (handoff)

> **Trạng thái: đã hoàn tất (v0.0.2).** `go build ./...`, `go vet ./...`,
> `gofmt -l internal` và toàn bộ `go test ./...` đều xanh; `internal/tui` có
> 22 test. Các mục 0–4 bên dưới đã xử lý xong — xem mục 6 và 7.

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

`tui_test.go` + `render_test.go`: 22 test phủ cage verdict, flash, toast,
confirm, palette, meter easing, render thuần theo thời điểm, và quét mọi
kích thước terminal × mọi view để chắc chắn không panic.

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

## 7) Việc còn lại (chưa làm)

- `Runner` chưa được nối vào `internal/shell`: cần một implementation thật để
  `sbt` chạy TUI thay cho REPL hiện tại (xem `RunSession`).
- Diff viewer cho `changesView` (hiện chỉ liệt kê, chưa xem nội dung diff).
- `docs/LIMITATIONS.md` được tham chiếu trong thông báo overlayfs nhưng chưa
  tồn tại.

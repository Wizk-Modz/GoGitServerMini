# Quy Tắc Phát Triển

Task chỉ hoàn thành khi đã self-review kỹ và khắc phục toàn bộ lỗi.

## 1. Quy trình
**PLAN → BRANCH → IMPLEMENT → SELF-REVIEW → FIX → VERIFY → COMMIT → REPORT**
- Task nhỏ (typo, sửa 1 dòng, docs): bỏ PLAN/BRANCH, vẫn bắt buộc SELF-REVIEW → VERIFY.
- Chia task thành các phần logic; xong phần nào (đã self-review và verify) thì commit phần đó ngay.
- Chỉ tạo branch khi thư mục là git repo; chưa init → hỏi trước khi `git init`.
- Sửa bug: tái hiện lỗi trước, sửa đúng root cause (không vá triệu chứng), kiểm tra lại các flow liên quan.
- Không chỉ báo cáo lỗi mà không sửa. Không thay đổi ngoài scope task.
- Lỗi không sửa được → rollback, thông báo lý do. Không để code nửa chừng.
- Task quá lớn → đề xuất tách nhỏ. Dependency xung đột → báo cụ thể version.

## 2. Dự án mới → MVP (Minimum Viable Product)
- Chốt phạm vi MVP trước: bản nhỏ nhất chạy được end-to-end, chỉ gồm chức năng cốt lõi giải quyết đúng bài toán chính.
- Chưa xong MVP thì không thêm tính năng phụ, thư viện hay tối ưu sớm (tránh over-engineering).
- Phạm vi quá lớn → tách thành nhiều MVP nhỏ, bàn giao theo từng iteration.

## 3. Comment & Đặt tên
- Hàm public hoặc hàm có logic không hiển nhiên: bắt buộc **1 dòng comment** ngay trên định nghĩa, mô tả ngắn gọn chức năng. Getter/setter, override đơn giản, hàm một dòng hiển nhiên: miễn.
- Comment phải ngắn gọn và chỉ mô tả trực tiếp nội dung cần thiết. Không thêm phần tham chiếu như `(khớp app.py tham chiếu)`, `(theo app.py)`, `(xem file X)`, v.v. nếu không được yêu cầu.
- Dùng ký hiệu comment đúng ngôn ngữ (`#` Python/Ruby, `//` JS/TS/Java/Go/Rust/C/PHP, `--` SQL/Lua).
- **Comment tiếng Việt (hoặc ngôn ngữ có dấu) phải viết đầy đủ dấu** — tuyệt đối không viết không dấu.
  - ✅ Đúng: `# Thư mục gốc của repo (tools/ nằm trong đó)`
  - ❌ Sai: `# Thu muc goc cua repo (tools/ nam trong do)`
- Đặt tên theo convention chuẩn của ngôn ngữ đang dùng (snake_case, camelCase, PascalCase tương ứng).
- Tên rõ nghĩa, tránh viết tắt mơ hồ (trừ `i`, `ctx`, `err`, `req`, `res`, `db`, `tx`).
- **Ưu tiên convention dự án hiện có** hơn convention mặc định.

## 4. Cấu trúc & Dependency
- Tạo file/thư mục theo cấu trúc chuẩn của ngôn ngữ. Dự án có sẵn cấu trúc → tuân theo, không tự ý tổ chức lại.
- Không tự ý thêm/nâng cấp dependency. Ghi rõ version, tránh `latest`.
- Lock file phải đồng bộ. Kiểm tra tương thích trước khi thêm dependency mới.
- File > 300 dòng → cân nhắc tách module. Sửa một phần → dùng chỉnh sửa cục bộ.

## 5. Review & Checklist
- **Chức năng**: Logic, edge cases, null/exception, nhất quán trạng thái.
- **Chất lượng**: Không dead code / unused import, đặt tên chuẩn.
- **Bảo mật**: Validate input, không hardcode secrets, tránh leak tài nguyên.
- **Tương thích**: Không phá vỡ API/interface hiện có.

Checklist:
- [ ] Build/lint/typecheck OK
- [ ] Không broken import, không sót implementation
- [ ] Không TODO/FIXME còn sót (trừ khi yêu cầu)
- [ ] Không còn debug log / print tạm
- [ ] Tests pass, config hợp lệ
- [ ] Không hardcode secrets, input đã validate
- [ ] Không phá vỡ API/interface hiện có
- [ ] Không xoá/ghi đè file ngoài scope
- [ ] Dependency không xung đột, lock file đồng bộ

## 6. Build & Test
- Chạy build/lint/test bằng tool đã cấu hình trong dự án — không tự thêm tool mới.
- Dự án không có test/lint → không tự ý cài, nhưng **phải đề xuất** 2-4 lựa chọn test phù hợp stack cho user chọn:
  - Mỗi lựa chọn nêu rõ: loại test, công cụ đề xuất kèm version, và những gì sẽ cài vào hệ thống.
  - Ưu tiên phương án đơn giản nhất, ít phụ thuộc nhất và không cần cài thêm vào máy (ví dụ JavaScript: `node:test` tích hợp sẵn trong Node, không cần dependency; Vitest/Jest chỉ đề xuất khi dự án thực sự cần).
  - Xếp từ dễ → phức tạp: unit → integration → e2e.
  - User từ chối hoặc bỏ qua → ghi nhận trong phần "Rủi ro còn lại" của báo cáo rằng dự án chưa có test.
  - User đồng ý → ghi rõ version, đồng bộ lock file, chạy thử test đầu tiên cho đến khi pass.

## 7. Báo cáo cuối (bắt buộc)
```
## Báo cáo: <tên task ngắn>

**Trạng thái:** DONE | PARTIAL | BLOCKED

**Đã thay đổi:**
- `path/file`: mô tả ngắn (+X/−Y dòng)

**Đã kiểm tra:**
- `path/file` — lý do kiểm tra

**Lỗi phát hiện:** N
- [đã sửa] mô tả lỗi → cách sửa
- [chưa sửa] mô tả lỗi → lý do / đề xuất

**Kiểm chứng:**
- `<lệnh đã chạy>` → kết quả thực tế (số liệu, exit code)
- UI (nếu có): đã thao tác gì trên trình duyệt, route nào, viewport nào

**Rủi ro còn lại:** None | <danh sách cụ thể>

**Commit:** <hash> <message> (mỗi phần một commit) | không commit (nêu lý do)
```

## 8. Commit Message
Tiền tố: `feat` | `fix` | `style` | `refactor` | `perf` | `test` | `docs` | `ci` | `chore`
- Định dạng: `<tiền tố>(<scope>): <mô tả ngắn ở thể mệnh lệnh>`.
- Ví dụ:
  - `feat(ui): thêm nút đăng xuất ở thanh header`
  - `fix(cli): sửa lỗi bỏ qua tham số --output`
  - `feat(ocr): thêm nhận dạng PDF nhiều trang`

## 9. Quy tắc cốt lõi
- **Commit theo từng phần**: làm xong phần nào commit phần đó ngay, mỗi commit là một thay đổi logic trọn vẹn, message theo mục 8.
- **Tuyệt đối không commit secrets/credentials.**
- **Theo style & tư duy dự án**: code theo đúng style, pattern, tư duy hiện có của dự án — chỉ thay đổi khi user yêu cầu rõ ràng.
- **File hướng dẫn AI agent**: đọc và tuân theo `CLAUDE.md` và `AGENTS.md` nếu dự án có. Xung đột giữa hai file → áp dụng `CLAUDE.md` trước, rồi báo user phần mâu thuẫn.
- **Đồng bộ file hướng dẫn**: dự án có cả hai file → sửa rule chung ở cả hai (trừ khi là symlink). Dự án chưa có file nào → **không tự tạo**, hỏi user nếu cần.
- **Không chắc chắn** → hỏi lại, không tự đoán.
- **Code phải hoạt động đúng chức năng, không chỉ compile được.** Luôn kiểm chứng logic, chạy thử hoặc trace qua flow để đảm bảo kết quả đúng — không chỉ dừng ở build/lint pass.

# GoGitServerMini

Git server tự host tối giản viết bằng Go: phục vụ `git clone/push/pull` qua **Smart HTTP**, có giao diện web để tạo, xem, sửa mô tả và xoá repo. Giao diện hỗ trợ sáng/tối theo hệ điều hành và có nút chuyển thủ công. Metadata repo lưu trong một file JSON.

## Yêu cầu

- Go 1.22+
- `git` trong `PATH` (cần `git-http-backend`)

## Chạy

```bash
go run . -b 127.0.0.1 -p 8080 -repos ./repos
```

Mở http://127.0.0.1:8080 để tạo repo, sau đó:

```bash
git clone http://127.0.0.1:8080/<tên-repo>.git
```

Cho máy khác trong LAN truy cập:

```bash
go run . -b 0.0.0.0 -p 8080
```

Đẩy một dự án có sẵn lên:

```bash
cd /đường/dẫn/dự-án
git init
git add .
git commit -m "commit đầu tiên"
git remote add origin http://127.0.0.1:8080/<tên-repo>.git
git push -u origin main
```

## Cờ dòng lệnh

| Cờ | Mặc định | Ý nghĩa |
|---|---|---|
| `-b` | `127.0.0.1` | Địa chỉ IP lắng nghe |
| `-p` | `8080` | Cổng lắng nghe |
| `-repos` | `./repos` | Thư mục chứa các bare repo |
| `-metadata` | `./repos/metadata.json` | File JSON lưu metadata repo |
| `-max-body` | `536870912` | Giới hạn body mỗi request git (byte) |

## Metadata

Mỗi repo có mô tả, thời điểm tạo và cập nhật lưu trong `metadata.json`:

```json
{
  "version": 1,
  "repos": {
    "demo": {
      "name": "demo",
      "description": "repo thử nghiệm",
      "created_at": "2026-11-07T10:00:00Z",
      "updated_at": "2026-11-07T10:05:00Z"
    }
  }
}
```

File được ghi qua file tạm rồi `rename` để không hỏng dữ liệu khi ghi dở. Sửa mô tả trực tiếp trên trang chi tiết repo.

## Kiến trúc

```
main.go                      khởi động server, log request, graceful shutdown
internal/repostore           tạo/list/xoá bare repo, validate tên, chỉnh HEAD
internal/gitbackend          CGI bridge sang git http-backend
internal/repoinfo            đọc nhánh, tag, commit qua git CLI
internal/metastore           metadata repo trong file JSON
internal/web                 route HTML + template nhúng
```

Request `/<repo>.git/*` được chuyển thẳng cho `git http-backend` qua CGI. Các route còn lại do tầng web xử lý.

Sau mỗi lần push, server tự trỏ `HEAD` về nhánh duy nhất nếu `HEAD` đang trỏ nhánh chưa tồn tại — nhờ vậy repo chỉ có `master` (hoặc tên khác) vẫn clone ra đầy đủ.

## Kiểm thử

```bash
go test ./...
```

Test tích hợp dùng `httptest` + `git` thật để push rồi clone lại qua HTTP; tự bỏ qua nếu máy không có git.

## Giới hạn của MVP

- **Không có xác thực**: ai truy cập được cổng cũng clone/push/xoá được. Chỉ dùng trong mạng tin cậy, mặc định bind `127.0.0.1`.
- Không có SSH, pull request, issue, fork, LFS, phân quyền.
- Xoá repo là xoá vĩnh viễn, không có thùng rác.
- Metadata JSON ghi tuần tự toàn bộ file; phù hợp vài nghìn repo, chưa phải SQLite.
- Chưa có phân trang; trang chủ đọc commit gần nhất của từng repo nên rất nhiều repo sẽ chậm dần.

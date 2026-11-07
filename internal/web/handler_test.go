package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gogitservermini/internal/metastore"
	"gogitservermini/internal/repostore"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newHandler(t *testing.T) (*Handler, *repostore.Store, *metastore.Store) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}
	dir := t.TempDir()
	store, err := repostore.New(filepath.Join(dir, "repos"))
	if err != nil {
		t.Fatalf("repostore.New: %v", err)
	}
	meta, err := metastore.Open(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatalf("metastore.Open: %v", err)
	}
	handler, err := New(store, meta, 8<<20, discardLogger())
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}
	return handler, store, meta
}

func do(t *testing.T, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestIndexEmpty(t *testing.T) {
	h, _, _ := newHandler(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Chưa có repo nào") {
		t.Error("thiếu empty state")
	}
	if !strings.Contains(body, "Tạo repo mới") {
		t.Error("thiếu form tạo repo")
	}
}

func TestCreateRepo(t *testing.T) {
	h, store, _ := newHandler(t)
	form := url.Values{"name": {"demo"}}
	req := httptest.NewRequest(http.MethodPost, "/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := do(t, h.Routes(), req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/repo/demo" {
		t.Errorf("Location = %q", loc)
	}
	if !store.Exists("demo") {
		t.Error("repo chưa được tạo trên đĩa")
	}
}

func TestCreateRepoInvalidName(t *testing.T) {
	h, store, _ := newHandler(t)
	cases := []struct {
		name    string
		message string
	}{
		{"", "Vui lòng nhập tên repo"},
		{"../evil", "không hợp lệ"},
		{"a b", "không hợp lệ"},
		{"abc.git", "không hợp lệ"},
	}
	for _, tc := range cases {
		form := url.Values{"name": {tc.name}}
		req := httptest.NewRequest(http.MethodPost, "/new", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := do(t, h.Routes(), req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("name=%q status = %d, muốn 400", tc.name, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), tc.message) {
			t.Errorf("name=%q thiếu thông báo %q", tc.name, tc.message)
		}
	}
	if names, _ := store.List(); len(names) != 0 {
		t.Errorf("đã tạo repo ngoài ý muốn: %v", names)
	}
}

func TestCreateRepoDuplicate(t *testing.T) {
	h, _, _ := newHandler(t)
	post := func() *httptest.ResponseRecorder {
		form := url.Values{"name": {"demo"}}
		req := httptest.NewRequest(http.MethodPost, "/new", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return do(t, h.Routes(), req)
	}
	if rec := post(); rec.Code != http.StatusSeeOther {
		t.Fatalf("lần đầu status = %d", rec.Code)
	}
	rec := post()
	if rec.Code != http.StatusConflict {
		t.Errorf("lần hai status = %d, muốn 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "đã tồn tại") {
		t.Error("thiếu thông báo trùng tên")
	}
}

func TestRepoDetailEmpty(t *testing.T) {
	h, store, _ := newHandler(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "http://example.com/demo.git") {
		t.Errorf("thiếu clone URL:\n%s", body)
	}
	if !strings.Contains(body, "Repo này còn trống") {
		t.Error("thiếu hướng dẫn push lần đầu")
	}
}

func TestRepoDetailNotFound(t *testing.T) {
	h, _, _ := newHandler(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/khong-co", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, muốn 404", rec.Code)
	}
}

func TestDeleteRepo(t *testing.T) {
	h, store, _ := newHandler(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodPost, "/repo/demo/delete", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	if store.Exists("demo") {
		t.Error("repo vẫn còn sau khi xoá")
	}

	rec = do(t, h.Routes(), httptest.NewRequest(http.MethodPost, "/repo/demo/delete", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("xoá lần hai status = %d, muốn 404", rec.Code)
	}
}

func TestDeleteRequiresPost(t *testing.T) {
	h, store, _ := newHandler(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo/delete", nil))
	if rec.Code == http.StatusOK && !store.Exists("demo") {
		t.Fatal("repo bị xoá qua GET")
	}
	if !store.Exists("demo") {
		t.Error("repo bị xoá ngoài ý muốn")
	}
}

func TestIndexShowsRepoWithCommit(t *testing.T) {
	h, store, _ := newHandler(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	path, err := store.Path("demo")
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	pushCommit(t, path)

	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "commit đầu tiên") {
		t.Error("trang chủ thiếu commit gần nhất")
	}
	if strings.Contains(body, "Chưa có repo nào") {
		t.Error("trang chủ vẫn hiển thị empty state")
	}

	rec = do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo", nil))
	body = rec.Body.String()
	if !strings.Contains(body, "commit đầu tiên") {
		t.Error("trang chi tiết thiếu commit")
	}
	if !strings.Contains(body, "<code>main</code>") {
		t.Error("trang chi tiết thiếu nhánh main")
	}
}

func TestGitEndpointRouted(t *testing.T) {
	h, store, _ := newHandler(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/demo.git/info/refs?service=git-upload-pack", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-git-upload-pack-advertisement" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestUnknownPathIsHTML404(t *testing.T) {
	h, _, _ := newHandler(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/khong/ton/tai", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, muốn 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, muốn text/html", ct)
	}
}

func TestMissingGitRepoIsHTML404(t *testing.T) {
	h, _, _ := newHandler(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/khong-co.git/info/refs?service=git-upload-pack", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, muốn 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, muốn text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "không tồn tại") {
		t.Error("thiếu thông báo repo không tồn tại")
	}
}

func TestCreateRepoOversizedForm(t *testing.T) {
	h, store, _ := newHandler(t)
	form := url.Values{"name": {"demo"}, "padding": {strings.Repeat("x", maxFormBytes)}}
	req := httptest.NewRequest(http.MethodPost, "/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := do(t, h.Routes(), req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, muốn 400", rec.Code)
	}
	if names, _ := store.List(); len(names) != 0 {
		t.Errorf("đã tạo repo ngoài ý muốn: %v", names)
	}
}

func TestCreateRepoWithDescription(t *testing.T) {
	h, _, meta := newHandler(t)
	form := url.Values{"name": {"demo"}, "description": {"  repo thử nghiệm  "}}
	req := httptest.NewRequest(http.MethodPost, "/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := do(t, h.Routes(), req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	repo, ok := meta.Get("demo")
	if !ok {
		t.Fatal("metadata chưa được lưu")
	}
	if repo.Description != "repo thử nghiệm" {
		t.Errorf("Description = %q, muốn đã trim", repo.Description)
	}

	rec = do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo", nil))
	if !strings.Contains(rec.Body.String(), "repo thử nghiệm") {
		t.Error("trang chi tiết thiếu mô tả")
	}
	rec = do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rec.Body.String(), "repo thử nghiệm") {
		t.Error("trang chủ thiếu mô tả")
	}
}

func TestEditDescription(t *testing.T) {
	h, store, meta := newHandler(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	form := url.Values{"description": {"mô tả mới"}}
	req := httptest.NewRequest(http.MethodPost, "/repo/demo/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := do(t, h.Routes(), req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	repo, ok := meta.Get("demo")
	if !ok || repo.Description != "mô tả mới" {
		t.Errorf("metadata = %+v ok=%v", repo, ok)
	}

	// Sửa lần hai về rỗng phải xoá nội dung cũ.
	req = httptest.NewRequest(http.MethodPost, "/repo/demo/edit", strings.NewReader("description="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	do(t, h.Routes(), req)
	repo, _ = meta.Get("demo")
	if repo.Description != "" {
		t.Errorf("Description = %q, muốn rỗng", repo.Description)
	}
}

func TestEditDescriptionTooLong(t *testing.T) {
	h, store, _ := newHandler(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	form := url.Values{"description": {strings.Repeat("a", maxDescriptionBytes+1)}}
	req := httptest.NewRequest(http.MethodPost, "/repo/demo/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := do(t, h.Routes(), req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, muốn 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "quá dài") {
		t.Error("thiếu thông báo mô tả quá dài")
	}
}

func TestEditMissingRepo(t *testing.T) {
	h, _, _ := newHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/repo/ghost/edit", strings.NewReader("description=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := do(t, h.Routes(), req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, muốn 404", rec.Code)
	}
}

func TestDeleteRemovesMetadata(t *testing.T) {
	h, store, meta := newHandler(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := meta.Set("demo", "sẽ bị xoá"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	do(t, h.Routes(), httptest.NewRequest(http.MethodPost, "/repo/demo/delete", nil))
	if _, ok := meta.Get("demo"); ok {
		t.Error("metadata vẫn còn sau khi xoá repo")
	}
}

func TestRepoNameEscaped(t *testing.T) {
	h, store, _ := newHandler(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "<script>") {
		t.Error("có thẻ script chưa escape")
	}
}

func pushCommit(t *testing.T, bareRepo string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	run(t, "", "git", "clone", bareRepo, work)
	run(t, work, "git", "config", "user.name", "Tester")
	run(t, work, "git", "config", "user.email", "tester@example.com")
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, work, "git", "add", ".")
	run(t, work, "git", "commit", "-m", "commit đầu tiên")
	run(t, work, "git", "push", "origin", "HEAD:main")
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v lỗi: %v\n%s", name, args, err, out)
	}
}

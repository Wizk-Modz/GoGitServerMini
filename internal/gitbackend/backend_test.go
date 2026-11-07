package gitbackend

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gogitservermini/internal/repostore"
)

func newTestStore(t *testing.T) *repostore.Store {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}
	store, err := repostore.New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSplitRepoPath(t *testing.T) {
	cases := []struct {
		path     string
		name     string
		pathInfo string
		ok       bool
	}{
		{"/demo.git/info/refs", "demo", "/demo.git/info/refs", true},
		{"/demo.git/git-upload-pack", "demo", "/demo.git/git-upload-pack", true},
		{"/demo/info/refs", "", "", false},
		{"/.git/info/refs", "", "", false},
		{"/../evil.git/info/refs", "", "", false},
		{"/demo.git", "", "", false},
		{"/", "", "", false},
		{"/demo.git/a/b", "demo", "/demo.git/a/b", true},
	}
	for _, tc := range cases {
		name, pathInfo, ok := splitRepoPath(tc.path)
		if ok != tc.ok || name != tc.name || pathInfo != tc.pathInfo {
			t.Errorf("splitRepoPath(%q) = (%q, %q, %v), muốn (%q, %q, %v)",
				tc.path, name, pathInfo, ok, tc.name, tc.pathInfo, tc.ok)
		}
	}
}

func TestServeHTTPUnknownRepo(t *testing.T) {
	store := newTestStore(t)
	handler := New(store, 1<<20, discardLogger())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing.git/info/refs?service=git-upload-pack", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, muốn 404", rec.Code)
	}
}

func TestServeHTTPInvalidPath(t *testing.T) {
	store := newTestStore(t)
	handler := New(store, 1<<20, discardLogger())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/no-suffix/info/refs", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, muốn 404", rec.Code)
	}
}

func TestServeHTTPInfoRefs(t *testing.T) {
	store := newTestStore(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	handler := New(store, 1<<20, discardLogger())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/demo.git/info/refs?service=git-upload-pack", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/x-git-upload-pack-advertisement" {
		t.Errorf("Content-Type = %q", got)
	}
	if !strings.HasPrefix(rec.Body.String(), "001e# service=git-upload-pack") {
		t.Errorf("body không đúng định dạng smart HTTP: %q", rec.Body.String())
	}
}

func TestServeHTTPBodyTooLarge(t *testing.T) {
	store := newTestStore(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	handler := New(store, 16, discardLogger())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/demo.git/git-receive-pack", strings.NewReader(strings.Repeat("x", 64)))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, muốn 413", rec.Code)
	}
}

func TestServeHTTPChunkedBodyRejected(t *testing.T) {
	store := newTestStore(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	handler := New(store, 1<<20, discardLogger())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/demo.git/git-receive-pack", strings.NewReader("data"))
	req.ContentLength = -1
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusLengthRequired {
		t.Errorf("status = %d, muốn 411", rec.Code)
	}
}

// TestCloneAndPush kiểm tra trọn vòng đời push rồi clone qua HTTP.
func TestCloneAndPush(t *testing.T) {
	store := newTestStore(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	handler := New(store, 32<<20, discardLogger())
	server := httptest.NewServer(handler)
	defer server.Close()

	work := t.TempDir()
	run(t, work, "git", "init", "-b", "main")
	run(t, work, "git", "config", "user.name", "Tester")
	run(t, work, "git", "config", "user.email", "tester@example.com")
	run(t, work, "git", "config", "http.receivepack", "true")
	writeFile(t, filepath.Join(work, "README.md"), "xin chào\n")
	run(t, work, "git", "add", ".")
	run(t, work, "git", "commit", "-m", "commit đầu tiên")
	run(t, work, "git", "remote", "add", "origin", server.URL+"/demo.git")
	run(t, work, "git", "push", "origin", "main")

	cloneDir := filepath.Join(t.TempDir(), "clone")
	run(t, "", "git", "clone", server.URL+"/demo.git", cloneDir)
	if got := readFile(t, filepath.Join(cloneDir, "README.md")); got != "xin chào\n" {
		t.Errorf("nội dung clone = %q", got)
	}

	writeFile(t, filepath.Join(work, "second.txt"), "commit thứ hai\n")
	run(t, work, "git", "add", ".")
	run(t, work, "git", "commit", "-m", "commit thứ hai")
	run(t, work, "git", "push", "origin", "main")

	run(t, cloneDir, "git", "pull", "origin", "main")
	if got := readFile(t, filepath.Join(cloneDir, "second.txt")); got != "commit thứ hai\n" {
		t.Errorf("nội dung sau pull = %q", got)
	}
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("ghi %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc %s: %v", path, err)
	}
	return string(data)
}

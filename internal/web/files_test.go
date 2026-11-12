package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gogitservermini/internal/repostore"
)

// pushFilesRepo tạo repo có README, tệp lồng trong thư mục và ảnh, rồi push lên bare repo.
func pushFilesRepo(t *testing.T, bareRepo string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	run(t, "", "git", "clone", bareRepo, work)
	run(t, work, "git", "config", "user.name", "Tester")
	run(t, work, "git", "config", "user.email", "tester@example.com")
	writeTestFile(t, filepath.Join(work, "README.md"), "# Xin chào\n\nXem [tài liệu](docs/GUIDE.md).\n\n![logo](logo.png)\n")
	writeTestFile(t, filepath.Join(work, "docs", "GUIDE.md"), "# Hướng dẫn\n\nQuay lại [README](../README.md).\n")
	writeTestFile(t, filepath.Join(work, "docs", "a.txt"), "hello\n")
	writeTestFile(t, filepath.Join(work, "logo.png"), pngBytes())
	run(t, work, "git", "add", ".")
	run(t, work, "git", "commit", "-m", "thêm tệp")
	run(t, work, "git", "push", "origin", "HEAD:main")
}

// pngBytes trả nội dung PNG 1x1 hợp lệ để kiểm thử nhận diện ảnh.
func pngBytes() string {
	return "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x02\x00\x00\x00\x90wS\xde" +
		"\x00\x00\x00\x0cIDATx\x9cc\xf8\xcf\xc0\x00\x00\x03\x01\x01\x00\x18\xdd\x8d\xb0\x00\x00\x00\x00IEND\xaeB`\x82"
}

// writeTestFile ghi tệp trong test, tự tạo thư mục cha.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("tạo thư mục: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("ghi %s: %v", path, err)
	}
}

// newFilesRepo dựng handler kèm repo "demo" đã có tệp.
func newFilesRepo(t *testing.T) (*Handler, *repostore.Store) {
	t.Helper()
	h, store, _ := newHandler(t)
	if err := store.Create("demo"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	path, err := store.Path("demo")
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	pushFilesRepo(t, path)
	return h, store
}

func TestRepoOverviewShowsFilesAndReadme(t *testing.T) {
	h, _ := newFilesRepo(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`class="panel repo-readme"`,
		"Xin chào",
		`href="/repo/demo/blob/docs/GUIDE.md?ref=main"`,
		`src="/repo/demo/raw/logo.png?ref=main"`,
		`href="/repo/demo/tree/docs?ref=main"`,
		`href="/repo/demo/tree/?ref=main"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("trang tổng quan thiếu %q", want)
		}
	}
}

func TestTreeListsEntries(t *testing.T) {
	h, _ := newFilesRepo(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo/tree/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`href="/repo/demo/tree/docs?ref=main"`,
		`href="/repo/demo/blob/README.md?ref=main"`,
		`href="/repo/demo/blob/logo.png?ref=main"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("trang duyệt tệp thiếu %q", want)
		}
	}
}

func TestTreeOnFileRedirectsToBlob(t *testing.T) {
	h, _ := newFilesRepo(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo/tree/README.md", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, muốn 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/repo/demo/blob/README.md?ref=main" {
		t.Errorf("Location = %q", loc)
	}
}

func TestBlobOnDirectoryRedirectsToTree(t *testing.T) {
	h, _ := newFilesRepo(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo/blob/docs", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, muốn 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/repo/demo/tree/docs?ref=main" {
		t.Errorf("Location = %q", loc)
	}
}

func TestBlobRendersMarkdownWithRepoLinks(t *testing.T) {
	h, _ := newFilesRepo(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo/blob/docs/GUIDE.md", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Hướng dẫn") {
		t.Error("thiếu nội dung Markdown")
	}
	if !strings.Contains(body, `href="/repo/demo/blob/README.md?ref=main"`) {
		t.Error("link tương đối chưa đổi sang URL trong repo")
	}
	if strings.Contains(body, `href="../README.md"`) {
		t.Error("vẫn còn link tương đối")
	}
}

func TestBlobRendersTextWithLineNumbers(t *testing.T) {
	h, _ := newFilesRepo(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo/blob/docs/a.txt", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`class="code-table"`, "hello"} {
		if !strings.Contains(body, want) {
			t.Errorf("khung xem code thiếu %q", want)
		}
	}
}

func TestBlobNotFound(t *testing.T) {
	h, _ := newFilesRepo(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo/blob/khong-co.txt", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, muốn 404", rec.Code)
	}
}

func TestRawServesContent(t *testing.T) {
	h, _ := newFilesRepo(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo/raw/docs/a.txt", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Body.String(); got != "hello\n" {
		t.Errorf("body = %q", got)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q", ct)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); csp != "sandbox" {
		t.Errorf("Content-Security-Policy = %q", csp)
	}
}

func TestRawServesImageContentType(t *testing.T) {
	h, _ := newFilesRepo(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo/raw/logo.png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, muốn image/png", ct)
	}
}

func TestRawServesFileLargerThanPreviewLimit(t *testing.T) {
	h, store := newFilesRepo(t)
	path, err := store.Path("demo")
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	work := filepath.Join(t.TempDir(), "work")
	run(t, "", "git", "clone", path, work)
	run(t, work, "git", "config", "user.name", "Tester")
	run(t, work, "git", "config", "user.email", "tester@example.com")
	writeTestFile(t, filepath.Join(work, "big.txt"), strings.Repeat("a", (1<<20)+10))
	run(t, work, "git", "add", ".")
	run(t, work, "git", "commit", "-m", "tệp lớn")
	run(t, work, "git", "push", "origin", "HEAD:main")

	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo/raw/big.txt", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, muốn 200", rec.Code)
	}
	if got := rec.Body.Len(); got != (1<<20)+10 {
		t.Errorf("độ dài body = %d, muốn %d", got, (1<<20)+10)
	}

	rec = do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo/blob/big.txt", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("trang blob status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Tệp quá lớn") {
		t.Error("trang blob thiếu thông báo tệp quá lớn")
	}
}

func TestBlobShowsImagePage(t *testing.T) {
	h, _ := newFilesRepo(t)
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/demo/blob/logo.png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, muốn text/html", ct)
	}
	for _, want := range []string{`class="blob-image"`, `src="/repo/demo/raw/logo.png?ref=main"`} {
		if !strings.Contains(body, want) {
			t.Errorf("trang xem ảnh thiếu %q", want)
		}
	}
}

func TestRawRejectsDirectoryAndMissingFile(t *testing.T) {
	h, _ := newFilesRepo(t)
	for _, target := range []string{"/repo/demo/raw/docs", "/repo/demo/raw/khong-co.txt"} {
		rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, muốn 404", target, rec.Code)
		}
	}
}

func TestFileURLsEscapeSpecialNames(t *testing.T) {
	cases := []struct {
		action, path, want string
	}{
		{"blob", "a%b", "/repo/demo/blob/a%25b"},
		{"blob", "a b/c#d.txt", "/repo/demo/blob/a%20b/c%23d.txt"},
		{"tree", "thư mục", "/repo/demo/tree/th%C6%B0%20m%E1%BB%A5c"},
		{"raw", "-x", "/repo/demo/raw/-x"},
	}
	for _, c := range cases {
		var got string
		switch c.action {
		case "blob":
			got = blobURL("demo", c.path, "")
		case "tree":
			got = treeURL("demo", c.path, "")
		case "raw":
			got = rawURL("demo", c.path, "")
		}
		if got != c.want {
			t.Errorf("%s(%q) = %q, muốn %q", c.action, c.path, got, c.want)
		}
	}
}

func TestResolveMarkdownURL(t *testing.T) {
	cases := []struct {
		name, baseDir, dest, want string
		image                     bool
	}{
		{name: "tệp cùng thư mục", baseDir: "docs", dest: "a.txt", want: "/repo/demo/blob/docs/a.txt?ref=main"},
		{name: "lên thư mục cha", baseDir: "docs", dest: "../README.md", want: "/repo/demo/blob/README.md?ref=main"},
		{name: "thư mục gốc", baseDir: "docs", dest: "../", want: "/repo/demo/tree/?ref=main"},
		{name: "ảnh tương đối", baseDir: "docs", dest: "../logo.png", want: "/repo/demo/raw/logo.png?ref=main", image: true},
		{name: "đường dẫn tuyệt đối", baseDir: "docs", dest: "/README.md", want: "/repo/demo/blob/README.md?ref=main"},
		{name: "giữ fragment", baseDir: "", dest: "a.txt#phan-2", want: "/repo/demo/blob/a.txt?ref=main#phan-2"},
		{name: "giữ query", baseDir: "", dest: "a.txt?x=1", want: "/repo/demo/blob/a.txt?ref=main&x=1"},
		{name: "bỏ qua link ngoài", baseDir: "docs", dest: "https://example.com/x", want: ""},
		{name: "bỏ qua anchor", baseDir: "docs", dest: "#muc-1", want: ""},
		{name: "bỏ qua scheme lạ", baseDir: "docs", dest: "javascript:alert(1)", want: ""},
		{name: "bỏ qua thoát repo", baseDir: "", dest: "../../etc/passwd", want: ""},
	}
	for _, c := range cases {
		got := resolveMarkdownURL("demo", "main", c.baseDir, c.dest, c.image)
		if got != c.want {
			t.Errorf("%s: resolveMarkdownURL(%q) = %q, muốn %q", c.name, c.dest, got, c.want)
		}
	}
}

func TestTreeEmptyRepoShows404(t *testing.T) {
	h, store, _ := newHandler(t)
	if err := store.Create("empty"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	rec := do(t, h.Routes(), httptest.NewRequest(http.MethodGet, "/repo/empty/tree/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, muốn 404", rec.Code)
	}
}

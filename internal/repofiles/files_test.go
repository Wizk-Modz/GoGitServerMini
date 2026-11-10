package repofiles

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// makeRepo tạo bare repo có sẵn cây tệp để kiểm thử.
func makeRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}
	dir := t.TempDir()
	bare := filepath.Join(dir, "demo.git")
	run(t, "", "git", "init", "--bare", "-b", "main", bare)

	work := filepath.Join(dir, "work")
	run(t, "", "git", "clone", bare, work)
	run(t, work, "git", "config", "user.name", "Tester")
	run(t, work, "git", "config", "user.email", "tester@example.com")
	writeFile(t, filepath.Join(work, "README.md"), "# Xin chào\n")
	writeFile(t, filepath.Join(work, "empty.txt"), "")
	writeFile(t, filepath.Join(work, "logo.bin"), string([]byte{0, 1, 2, 3}))
	mkdir(t, filepath.Join(work, "docs"))
	writeFile(t, filepath.Join(work, "docs", "a.txt"), "nội dung a\n")
	mkdir(t, filepath.Join(work, "docs", "sub"))
	writeFile(t, filepath.Join(work, "docs", "sub", "b.txt"), "nội dung b\n")
	run(t, work, "git", "add", ".")
	run(t, work, "git", "commit", "-m", "commit đầu tiên")
	run(t, work, "git", "push", "origin", "main")
	return bare
}

func TestTreeRoot(t *testing.T) {
	repo := makeRepo(t)
	entries, err := Tree(repo, "main", "")
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("số mục = %d, muốn 4: %+v", len(entries), entries)
	}
	byName := map[string]Entry{}
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	if !byName["docs"].IsDir {
		t.Error("docs phải là thư mục")
	}
	if byName["docs"].Path != "docs" {
		t.Errorf("Path(docs) = %q", byName["docs"].Path)
	}
	if byName["README.md"].IsDir {
		t.Error("README.md không phải thư mục")
	}
	if byName["README.md"].Size != int64(len("# Xin chào\n")) {
		t.Errorf("Size(README.md) = %d", byName["README.md"].Size)
	}
	if byName["README.md"].Hash == "" {
		t.Error("thiếu hash")
	}
	if byName["empty.txt"].Size != 0 {
		t.Errorf("Size(empty.txt) = %d", byName["empty.txt"].Size)
	}
}

func TestTreeSubdir(t *testing.T) {
	repo := makeRepo(t)
	entries, err := Tree(repo, "main", "docs")
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("số mục = %d, muốn 2: %+v", len(entries), entries)
	}
	for _, entry := range entries {
		if entry.Name == "sub" {
			if !entry.IsDir || entry.Path != "docs/sub" {
				t.Errorf("mục sub sai: %+v", entry)
			}
		}
		if entry.Name == "a.txt" && entry.Path != "docs/a.txt" {
			t.Errorf("Path(a.txt) = %q", entry.Path)
		}
	}
}

func TestReadTextFile(t *testing.T) {
	repo := makeRepo(t)
	blob, err := Read(repo, "main", "README.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(blob.Data) != "# Xin chào\n" {
		t.Errorf("Data = %q", blob.Data)
	}
	if blob.Binary {
		t.Error("tệp văn bản bị nhận nhầm là binary")
	}
	if blob.Size != int64(len(blob.Data)) {
		t.Errorf("Size = %d, len = %d", blob.Size, len(blob.Data))
	}
}

func TestReadBinaryFile(t *testing.T) {
	repo := makeRepo(t)
	blob, err := Read(repo, "main", "logo.bin")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !blob.Binary {
		t.Error("tệp nhị phân không được nhận diện")
	}
}

func TestReadNotFound(t *testing.T) {
	repo := makeRepo(t)
	for _, path := range []string{"khong-co.txt", "docs/khong-co.txt"} {
		if _, err := Read(repo, "main", path); !errors.Is(err, ErrNotFound) {
			t.Errorf("Read(%q) = %v, muốn ErrNotFound", path, err)
		}
	}
}

func TestReadDirectory(t *testing.T) {
	repo := makeRepo(t)
	if _, err := Read(repo, "main", "docs"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Read(docs) = %v, muốn ErrNotFound", err)
	}
}

func TestReadTooLarge(t *testing.T) {
	repo := makeRepo(t)
	work := filepath.Join(filepath.Dir(repo), "work")
	big := make([]byte, MaxBlobBytes+1)
	writeFile(t, filepath.Join(work, "big.txt"), string(big))
	run(t, work, "git", "add", ".")
	run(t, work, "git", "commit", "-m", "tệp lớn")
	run(t, work, "git", "push", "origin", "main")

	blob, err := Read(repo, "main", "big.txt")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Read = %v, muốn ErrTooLarge", err)
	}
	if blob.Size <= MaxBlobBytes {
		t.Errorf("Size = %d", blob.Size)
	}
}

func TestInvalidPath(t *testing.T) {
	repo := makeRepo(t)
	invalid := []string{"/etc/passwd", "../x", "docs/../README.md", "docs//a.txt", "./README.md", "docs/"}
	for _, path := range invalid {
		if _, err := Read(repo, "main", path); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("Read(%q) = %v, muốn ErrInvalidPath", path, err)
		}
		if _, err := Tree(repo, "main", path); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("Tree(%q) = %v, muốn ErrInvalidPath", path, err)
		}
	}
}

func TestInvalidRef(t *testing.T) {
	repo := makeRepo(t)
	invalid := []string{"", "-main", "main..x", "main:x", "a b", "HEAD~1"}
	for _, ref := range invalid {
		if _, err := Tree(repo, ref, ""); err == nil {
			t.Errorf("Tree(ref=%q) = nil, muốn lỗi", ref)
		}
	}
}

func TestMissingRef(t *testing.T) {
	repo := makeRepo(t)
	if _, err := Tree(repo, "khong-co", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("Tree = %v, muốn ErrNotFound", err)
	}
}

func TestType(t *testing.T) {
	repo := makeRepo(t)
	cases := map[string]string{"": "tree", "docs": "tree", "README.md": "blob"}
	for target, want := range cases {
		got, err := Type(repo, "main", target)
		if err != nil {
			t.Errorf("Type(%q): %v", target, err)
			continue
		}
		if got != want {
			t.Errorf("Type(%q) = %q, muốn %q", target, got, want)
		}
	}
	if _, err := Type(repo, "main", "khong-co"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Type(khong-co) = %v, muốn ErrNotFound", err)
	}
}

func TestEmptyRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}
	bare := filepath.Join(t.TempDir(), "empty.git")
	run(t, "", "git", "init", "--bare", "-b", "main", bare)
	if _, err := Tree(bare, "main", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("Tree = %v, muốn ErrNotFound", err)
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

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("tạo %s: %v", path, err)
	}
}

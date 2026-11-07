package repoinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// makeRepo tạo bare repo tạm và trả đường dẫn của nó.
func makeRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}
	dir := filepath.Join(t.TempDir(), "demo.git")
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

// commitTo thêm một commit mới vào bare repo qua bản clone tạm.
func commitTo(t *testing.T, repo, message string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	run(t, "", "git", "clone", repo, work)
	run(t, work, "git", "config", "user.name", "Tester")
	run(t, work, "git", "config", "user.email", "tester@example.com")
	if err := os.WriteFile(filepath.Join(work, "file.txt"), []byte(message), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, work, "git", "add", ".")
	run(t, work, "git", "commit", "-m", message)
	run(t, work, "git", "push", "origin", "HEAD:main")
}

func TestReadEmptyRepo(t *testing.T) {
	repo := makeRepo(t)
	info := Read(repo, 0)
	if info.Err != nil {
		t.Fatalf("Err = %v", info.Err)
	}
	if !info.Empty {
		t.Error("Empty = false, muốn true")
	}
	if len(info.Commits) != 0 || len(info.Branches) != 0 {
		t.Errorf("repo rỗng nhưng có dữ liệu: %+v", info)
	}
}

func TestReadWithCommit(t *testing.T) {
	repo := makeRepo(t)
	commitTo(t, repo, "commit đầu tiên")

	info := Read(repo, 0)
	if info.Err != nil {
		t.Fatalf("Err = %v", info.Err)
	}
	if info.Empty {
		t.Error("Empty = true, muốn false")
	}
	if len(info.Commits) != 1 {
		t.Fatalf("số commit = %d, muốn 1", len(info.Commits))
	}
	commit := info.Commits[0]
	if commit.Subject != "commit đầu tiên" {
		t.Errorf("Subject = %q", commit.Subject)
	}
	if commit.Author != "Tester" {
		t.Errorf("Author = %q", commit.Author)
	}
	if len(commit.Hash) != 7 {
		t.Errorf("Hash = %q, muốn 7 ký tự", commit.Hash)
	}
	if commit.Date.IsZero() {
		t.Error("Date chưa được parse")
	}
	if info.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, muốn main", info.DefaultBranch)
	}
	if len(info.Branches) != 1 || info.Branches[0] != "main" {
		t.Errorf("Branches = %v", info.Branches)
	}
}

func TestLastCommit(t *testing.T) {
	repo := makeRepo(t)
	if _, ok := LastCommit(repo); ok {
		t.Error("repo rỗng nhưng LastCommit trả ok = true")
	}
	commitTo(t, repo, "chỉ một commit")
	commit, ok := LastCommit(repo)
	if !ok {
		t.Fatal("LastCommit trả ok = false")
	}
	if commit.Subject != "chỉ một commit" {
		t.Errorf("Subject = %q", commit.Subject)
	}
}

func TestReadLimit(t *testing.T) {
	repo := makeRepo(t)
	commitTo(t, repo, "commit 1")
	commitTo(t, repo, "commit 2")
	info := Read(repo, 1)
	if info.Err != nil {
		t.Fatalf("Err = %v", info.Err)
	}
	if len(info.Commits) != 1 {
		t.Errorf("số commit = %d, muốn 1", len(info.Commits))
	}
}

func TestDefaultBranchFallsBackToExisting(t *testing.T) {
	repo := makeRepo(t)
	work := filepath.Join(t.TempDir(), "work")
	run(t, "", "git", "clone", repo, work)
	run(t, work, "git", "config", "user.name", "Tester")
	run(t, work, "git", "config", "user.email", "tester@example.com")
	if err := os.WriteFile(filepath.Join(work, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, work, "git", "add", ".")
	run(t, work, "git", "commit", "-m", "trên nhánh khác")
	run(t, work, "git", "push", "origin", "HEAD:develop")

	info := Read(repo, 0)
	if info.Err != nil {
		t.Fatalf("Err = %v", info.Err)
	}
	if info.DefaultBranch != "develop" {
		t.Errorf("DefaultBranch = %q, muốn develop", info.DefaultBranch)
	}
}

func TestReadMissingRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}
	info := Read(filepath.Join(t.TempDir(), "khong-ton-tai.git"), 0)
	if info.Err == nil {
		t.Error("Err = nil, muốn lỗi")
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

package repostore

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store
}

func TestValidateName(t *testing.T) {
	valid := []string{"demo", "a", "My-Repo_2", "repo.git2", "123abc"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v, muốn nil", name, err)
		}
	}

	invalid := []string{"", ".", "..", "../evil", "a b", "a/b", `a\b`, "-abc", "abc.git", "abc.GIT", "tên", "a.b/../c"}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil, muốn lỗi", name)
		}
	}
}

func TestCreateListDelete(t *testing.T) {
	store := newStore(t)

	if err := store.Create("beta"); err != nil {
		t.Fatalf("Create(beta): %v", err)
	}
	if err := store.Create("alpha"); err != nil {
		t.Fatalf("Create(alpha): %v", err)
	}

	names, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("List = %v, muốn [alpha beta]", names)
	}

	if !store.Exists("alpha") {
		t.Error("Exists(alpha) = false, muốn true")
	}

	if err := store.Delete("alpha"); err != nil {
		t.Fatalf("Delete(alpha): %v", err)
	}
	if store.Exists("alpha") {
		t.Error("repo vẫn tồn tại sau khi xoá")
	}
	if err := store.Delete("alpha"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete lần hai = %v, muốn ErrNotFound", err)
	}
}

func TestCreateDuplicate(t *testing.T) {
	store := newStore(t)
	if err := store.Create("dup"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Create("dup"); !errors.Is(err, ErrExists) {
		t.Errorf("Create trùng = %v, muốn ErrExists", err)
	}
}

func TestCreateInvalidName(t *testing.T) {
	store := newStore(t)
	if err := store.Create("../escape"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("Create(../escape) = %v, muốn ErrInvalidName", err)
	}
	entries, err := os.ReadDir(filepath.Dir(store.Root))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if entry.Name() == "escape.git" {
			t.Fatal("đã tạo thư mục ngoài Root")
		}
	}
}

func TestPathStaysInsideRoot(t *testing.T) {
	store := newStore(t)
	for _, name := range []string{"../x", "a/../../b", "..", ""} {
		if _, err := store.Path(name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Path(%q) = %v, muốn ErrInvalidName", name, err)
		}
	}
	full, err := store.Path("ok")
	if err != nil {
		t.Fatalf("Path(ok): %v", err)
	}
	if filepath.Dir(full) != store.Root {
		t.Errorf("Path(ok) = %q, muốn nằm trong %q", full, store.Root)
	}
}

func TestListIgnoresForeignEntries(t *testing.T) {
	store := newStore(t)
	if err := os.Mkdir(filepath.Join(store.Root, "not-a-repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "file.git"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	names, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("List = %v, muốn rỗng", names)
	}
}

func TestCreateConcurrent(t *testing.T) {
	store := newStore(t)
	const workers = 8
	var wg sync.WaitGroup
	results := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = store.Create("race")
		}(i)
	}
	wg.Wait()

	success, exists := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrExists):
			exists++
		default:
			t.Errorf("lỗi ngoài dự kiến: %v", err)
		}
	}
	if success != 1 || exists != workers-1 {
		t.Errorf("success=%d exists=%d, muốn 1 và %d", success, exists, workers-1)
	}
	if !store.Exists("race") {
		t.Error("repo không được tạo")
	}
}

func TestReceivePackEnabled(t *testing.T) {
	store := newStore(t)
	if err := store.Create("cfg"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	full, err := store.Path("cfg")
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	out, err := runGit("--git-dir", full, "config", "--get", "http.receivepack")
	if err != nil {
		t.Fatalf("đọc config: %v", err)
	}
	if out != "true" {
		t.Errorf("http.receivepack = %q, muốn true", out)
	}
}

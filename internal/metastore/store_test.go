package metastore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metadata.json")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return store, path
}

func TestOpenMissingFile(t *testing.T) {
	store, path := newStore(t)
	if names := store.Names(); len(names) != 0 {
		t.Errorf("Names = %v, muốn rỗng", names)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("không nên tạo file khi chưa ghi")
	}
}

func TestSetAndGet(t *testing.T) {
	store, _ := newStore(t)
	if err := store.Set("demo", "repo thử nghiệm"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	repo, ok := store.Get("demo")
	if !ok {
		t.Fatal("Get trả ok = false")
	}
	if repo.Description != "repo thử nghiệm" {
		t.Errorf("Description = %q", repo.Description)
	}
	if repo.CreatedAt.IsZero() || repo.UpdatedAt.IsZero() {
		t.Error("thiếu thời gian tạo/cập nhật")
	}
	if repo.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt không ở UTC: %v", repo.CreatedAt.Location())
	}
}

func TestPersistAcrossOpen(t *testing.T) {
	store, path := newStore(t)
	if err := store.Set("demo", "mô tả"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open lại: %v", err)
	}
	repo, ok := reopened.Get("demo")
	if !ok || repo.Description != "mô tả" {
		t.Errorf("sau khi mở lại: %+v ok=%v", repo, ok)
	}
}

func TestSetUpdatesExisting(t *testing.T) {
	store, _ := newStore(t)
	if err := store.Set("demo", "lần một"); err != nil {
		t.Fatal(err)
	}
	first, _ := store.Get("demo")

	if err := store.Set("demo", "lần hai"); err != nil {
		t.Fatal(err)
	}
	second, _ := store.Get("demo")
	if second.Description != "lần hai" {
		t.Errorf("Description = %q", second.Description)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Error("CreatedAt bị thay đổi khi cập nhật")
	}
}

func TestDelete(t *testing.T) {
	store, path := newStore(t)
	if err := store.Set("demo", "x"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("demo"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := store.Get("demo"); ok {
		t.Error("metadata vẫn còn sau khi xoá")
	}
	if err := store.Delete("khong-co"); err != nil {
		t.Errorf("Delete tên lạ = %v, muốn nil", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open lại: %v", err)
	}
	if _, ok := reopened.Get("demo"); ok {
		t.Error("metadata vẫn còn sau khi mở lại")
	}
}

func TestNamesSorted(t *testing.T) {
	store, _ := newStore(t)
	for _, name := range []string{"beta", "alpha", "gamma"} {
		if err := store.Set(name, ""); err != nil {
			t.Fatal(err)
		}
	}
	names := store.Names()
	want := []string{"alpha", "beta", "gamma"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("Names = %v, muốn %v", names, want)
		}
	}
}

func TestOpenInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.json")
	if err := os.WriteFile(path, []byte("{không phải json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Error("Open = nil, muốn lỗi")
	}
}

func TestOpenFutureVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"repos":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Error("Open = nil, muốn lỗi")
	}
}

func TestFileFormat(t *testing.T) {
	store, path := newStore(t)
	if err := store.Set("demo", "mô tả"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed fileData
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("file không phải JSON: %v", err)
	}
	if parsed.Version != schemaVersion {
		t.Errorf("version = %d", parsed.Version)
	}
	if parsed.Repos["demo"].Description != "mô tả" {
		t.Errorf("repos = %+v", parsed.Repos)
	}
}

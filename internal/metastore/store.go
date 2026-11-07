// Package metastore lưu metadata của repo trong một file JSON.
package metastore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// schemaVersion là phiên bản định dạng file metadata.
const schemaVersion = 1

// Repo là metadata của một repo.
type Repo struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// fileData là cấu trúc gốc của file metadata.
type fileData struct {
	Version int             `json:"version"`
	Repos   map[string]Repo `json:"repos"`
}

// Store đọc và ghi metadata, an toàn khi dùng đồng thời.
type Store struct {
	path  string
	mu    sync.RWMutex
	repos map[string]Repo
}

// Open nạp metadata từ path, tạo file rỗng nếu chưa có.
func Open(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("không xác định được đường dẫn metadata: %w", err)
	}
	store := &Store{path: abs, repos: make(map[string]Repo)}

	data, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("không đọc được metadata: %w", err)
	}
	if len(data) == 0 {
		return store, nil
	}

	var parsed fileData
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("metadata không phải JSON hợp lệ: %w", err)
	}
	if parsed.Version > schemaVersion {
		return nil, fmt.Errorf("metadata phiên bản %d chưa được hỗ trợ", parsed.Version)
	}
	for name, repo := range parsed.Repos {
		repo.Name = name
		store.repos[name] = repo
	}
	return store, nil
}

// Path trả đường dẫn file metadata.
func (s *Store) Path() string {
	return s.path
}

// Get trả metadata của repo, false nếu chưa có.
func (s *Store) Get(name string) (Repo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	repo, ok := s.repos[name]
	return repo, ok
}

// Set ghi mô tả cho repo, tạo mới nếu chưa có.
func (s *Store) Set(name, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Truncate(time.Second)
	repo, ok := s.repos[name]
	if !ok {
		repo = Repo{Name: name, CreatedAt: now}
	}
	repo.Name = name
	repo.Description = description
	repo.UpdatedAt = now
	s.repos[name] = repo
	return s.saveLocked()
}

// Delete xoá metadata của repo.
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.repos[name]; !ok {
		return nil
	}
	delete(s.repos, name)
	return s.saveLocked()
}

// Names trả tên các repo đã có metadata, sắp xếp theo bảng chữ cái.
func (s *Store) Names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.repos))
	for name := range s.repos {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// saveLocked ghi metadata xuống đĩa qua file tạm rồi rename.
func (s *Store) saveLocked() error {
	payload := fileData{Version: schemaVersion, Repos: s.repos}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("không mã hoá được metadata: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("không tạo được thư mục metadata: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".metadata-*.tmp")
	if err != nil {
		return fmt.Errorf("không tạo được file tạm: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("không ghi được metadata: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("không đóng được file tạm: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("không thay được file metadata: %w", err)
	}
	return nil
}

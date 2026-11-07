// Package repostore quản lý các bare repository nằm trong một thư mục gốc.
package repostore

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Các lỗi nghiệp vụ mà tầng web cần phân biệt.
var (
	ErrInvalidName = errors.New("tên repo không hợp lệ")
	ErrExists      = errors.New("repo đã tồn tại")
	ErrNotFound    = errors.New("repo không tồn tại")
)

// namePattern giới hạn tên repo ở ký tự an toàn cho URL và tên thư mục.
var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// Store quản lý các repo bare dưới thư mục Root.
type Store struct {
	Root string
}

// New tạo Store với thư mục gốc cho trước.
func New(root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("không xác định được đường dẫn repo: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("không tạo được thư mục repo: %w", err)
	}
	return &Store{Root: abs}, nil
}

// ValidateName kiểm tra tên repo có an toàn để dùng làm tên thư mục hay không.
func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return ErrInvalidName
	}
	if strings.HasSuffix(strings.ToLower(name), ".git") {
		return ErrInvalidName
	}
	return nil
}

// path trả đường dẫn tuyệt đối của repo, đảm bảo nằm trong Root.
func (s *Store) path(name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	full := filepath.Join(s.Root, name+".git")
	rel, err := filepath.Rel(s.Root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrInvalidName
	}
	return full, nil
}

// Path trả đường dẫn bare repo tương ứng với tên.
func (s *Store) Path(name string) (string, error) {
	return s.path(name)
}

// Exists cho biết repo đã tồn tại hay chưa.
func (s *Store) Exists(name string) bool {
	full, err := s.path(name)
	if err != nil {
		return false
	}
	info, err := os.Stat(full)
	return err == nil && info.IsDir()
}

// List trả tên các repo hiện có, sắp xếp theo bảng chữ cái.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return nil, fmt.Errorf("không đọc được thư mục repo: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name, ok := strings.CutSuffix(entry.Name(), ".git")
		if !ok || ValidateName(name) != nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// Create khởi tạo bare repo mới với tên cho trước.
func (s *Store) Create(name string) error {
	full, err := s.path(name)
	if err != nil {
		return err
	}
	// Mkdir đóng vai trò khoá nguyên tử để hai request đồng thời không cùng tạo một repo.
	if err := os.Mkdir(full, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrExists
		}
		return fmt.Errorf("không tạo được thư mục repo: %w", err)
	}
	if out, err := runGit("init", "--bare", full); err != nil {
		os.RemoveAll(full)
		return fmt.Errorf("git init thất bại: %w: %s", err, out)
	}
	if out, err := runGit("--git-dir", full, "symbolic-ref", "HEAD", "refs/heads/main"); err != nil {
		os.RemoveAll(full)
		return fmt.Errorf("không đặt được nhánh mặc định: %w: %s", err, out)
	}
	if _, err := runGit("--git-dir", full, "config", "http.receivepack", "true"); err != nil {
		os.RemoveAll(full)
		return fmt.Errorf("không bật được chế độ nhận push: %w", err)
	}
	return nil
}

// FixHead trỏ HEAD về nhánh duy nhất khi HEAD đang trỏ tới nhánh chưa tồn tại.
func (s *Store) FixHead(name string) error {
	full, err := s.path(name)
	if err != nil {
		return err
	}
	head, err := runGit("--git-dir", full, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return nil
	}
	head = strings.TrimPrefix(head, "refs/heads/")
	if head == "" {
		return nil
	}
	if _, err := runGit("--git-dir", full, "show-ref", "--verify", "--quiet", "refs/heads/"+head); err == nil {
		return nil
	}
	out, err := runGit("--git-dir", full, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return fmt.Errorf("không đọc được danh sách nhánh: %w", err)
	}
	branches := strings.Fields(out)
	if len(branches) != 1 {
		return nil
	}
	if _, err := runGit("--git-dir", full, "symbolic-ref", "HEAD", "refs/heads/"+branches[0]); err != nil {
		return fmt.Errorf("không cập nhật được HEAD: %w", err)
	}
	return nil
}

// Delete xoá hẳn thư mục repo khỏi đĩa.
func (s *Store) Delete(name string) error {
	full, err := s.path(name)
	if err != nil {
		return err
	}
	info, err := os.Stat(full)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("không kiểm tra được repo: %w", err)
	}
	if !info.IsDir() {
		return ErrNotFound
	}
	if err := os.RemoveAll(full); err != nil {
		return fmt.Errorf("không xoá được repo: %w", err)
	}
	return nil
}

// runGit chạy lệnh git và trả về thông báo lỗi gộp của git.
func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

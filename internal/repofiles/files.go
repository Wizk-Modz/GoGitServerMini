// Package repofiles đọc cây tệp và nội dung blob từ bare repo.
package repofiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// MaxBlobBytes giới hạn kích thước tệp được đọc để hiển thị.
const MaxBlobBytes = 1 << 20

// commandTimeout giới hạn thời gian mỗi lệnh git đọc tệp.
const commandTimeout = 10 * time.Second

// Các lỗi nghiệp vụ mà tầng web cần phân biệt.
var (
	ErrNotFound    = errors.New("không tìm thấy tệp")
	ErrTooLarge    = errors.New("tệp quá lớn")
	ErrInvalidPath = errors.New("đường dẫn không hợp lệ")
)

// Entry là một mục trong cây tệp của repo.
type Entry struct {
	Name  string
	Path  string
	Hash  string
	Size  int64
	IsDir bool
}

// Blob là nội dung một tệp kèm thông tin nhận diện.
type Blob struct {
	Data   []byte
	Size   int64
	Binary bool
}

// Tree liệt kê các mục trong thư mục dir của ref.
func Tree(repoPath, ref, dir string) ([]Entry, error) {
	cleanDir, err := cleanPath(dir)
	if err != nil {
		return nil, err
	}
	commit, err := resolveRef(repoPath, ref)
	if err != nil {
		return nil, err
	}
	treeish := commit
	if cleanDir != "" {
		treeish += ":" + cleanDir
	}
	out, err := runGit(repoPath, "ls-tree", "-z", "--long", treeish)
	if err != nil {
		return nil, ErrNotFound
	}
	return parseTree(out, cleanDir), nil
}

// Read đọc nội dung tệp tại filePath của ref.
func Read(repoPath, ref, filePath string) (Blob, error) {
	cleanFile, err := cleanPath(filePath)
	if err != nil {
		return Blob{}, err
	}
	if cleanFile == "" {
		return Blob{}, ErrInvalidPath
	}
	commit, err := resolveRef(repoPath, ref)
	if err != nil {
		return Blob{}, err
	}
	treeish := commit + ":" + cleanFile

	sizeOut, err := runGit(repoPath, "cat-file", "-s", treeish)
	if err != nil {
		return Blob{}, ErrNotFound
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(sizeOut)), 10, 64)
	if err != nil {
		return Blob{}, fmt.Errorf("không đọc được kích thước tệp: %w", err)
	}
	if size > MaxBlobBytes {
		return Blob{Size: size}, ErrTooLarge
	}

	data, err := runGit(repoPath, "cat-file", "blob", treeish)
	if err != nil {
		return Blob{}, ErrNotFound
	}
	head := data
	if len(head) > 8000 {
		head = head[:8000]
	}
	return Blob{Data: data, Size: size, Binary: bytes.IndexByte(head, 0) >= 0}, nil
}

// Reader trả luồng nội dung thô của tệp, dùng cho tải xuống nên không giới hạn kích thước.
func Reader(repoPath, ref, filePath string) (io.ReadCloser, error) {
	cleanFile, err := cleanPath(filePath)
	if err != nil {
		return nil, err
	}
	if cleanFile == "" {
		return nil, ErrInvalidPath
	}
	commit, err := resolveRef(repoPath, ref)
	if err != nil {
		return nil, err
	}
	treeish := commit + ":" + cleanFile
	// git cat-file báo lỗi thiếu tệp ở stderr khi kết thúc, nên xác nhận trước khi stream.
	if _, err := runGit(repoPath, "cat-file", "-s", treeish); err != nil {
		return nil, ErrNotFound
	}

	cmd := exec.Command("git", "--git-dir", repoPath, "cat-file", "blob", treeish)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("không mở được luồng git: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("không chạy được git cat-file: %w", err)
	}
	return &stream{pipe: stdout, cmd: cmd, stderr: &stderr}, nil
}

// stream bọc pipe stdout của git và thu hồi tiến trình khi đóng.
type stream struct {
	pipe   io.ReadCloser
	cmd    *exec.Cmd
	stderr *strings.Builder
}

// Read chuyển tiếp dữ liệu từ pipe của git.
func (s *stream) Read(p []byte) (int, error) {
	return s.pipe.Read(p)
}

// Close đóng pipe rồi chờ git kết thúc để không bỏ lại tiến trình.
func (s *stream) Close() error {
	err := s.pipe.Close()
	waitErr := s.cmd.Wait()
	if err != nil {
		return err
	}
	if waitErr != nil && s.stderr.Len() > 0 {
		return fmt.Errorf("git cat-file: %w: %s", waitErr, strings.TrimSpace(s.stderr.String()))
	}
	return nil
}

// Type trả loại đối tượng tại target: "blob" hoặc "tree".
func Type(repoPath, ref, target string) (string, error) {
	cleanTarget, err := cleanPath(target)
	if err != nil {
		return "", err
	}
	commit, err := resolveRef(repoPath, ref)
	if err != nil {
		return "", err
	}
	if cleanTarget == "" {
		return "tree", nil
	}
	out, err := runGit(repoPath, "cat-file", "-t", commit+":"+cleanTarget)
	if err != nil {
		return "", ErrNotFound
	}
	switch kind := strings.TrimSpace(string(out)); kind {
	case "blob", "tree":
		return kind, nil
	default:
		return "", ErrNotFound
	}
}

// resolveRef đổi tên nhánh/tag thành commit hash để dùng nhất quán trong request.
func resolveRef(repoPath, ref string) (string, error) {
	if err := validateRef(ref); err != nil {
		return "", err
	}
	out, err := runGit(repoPath, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", ErrNotFound
	}
	commit := strings.TrimSpace(string(out))
	if commit == "" {
		return "", ErrNotFound
	}
	return commit, nil
}

// validateRef từ chối tên ref có ký tự có thể gây nhập nhằng khi ghép lệnh git.
func validateRef(ref string) error {
	if ref == "" || len(ref) > 255 || strings.HasPrefix(ref, "-") {
		return ErrNotFound
	}
	if strings.Contains(ref, "..") || strings.ContainsAny(ref, " \t\r\n\x00:~^?*[\\") {
		return ErrNotFound
	}
	return nil
}

// cleanPath chuẩn hoá đường dẫn tương đối và chặn thoát khỏi thư mục gốc.
func cleanPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if strings.HasPrefix(p, "/") || strings.ContainsRune(p, 0) {
		return "", ErrInvalidPath
	}
	parts := strings.Split(p, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", ErrInvalidPath
		}
	}
	return strings.Join(parts, "/"), nil
}

// parseTree đọc output NUL-separated của git ls-tree --long.
func parseTree(out []byte, dir string) []Entry {
	var entries []Entry
	for _, record := range bytes.Split(out, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		meta, name, found := bytes.Cut(record, []byte{'\t'})
		if !found {
			continue
		}
		fields := strings.Fields(string(meta))
		if len(fields) < 4 {
			continue
		}
		entry := Entry{Name: string(name), Hash: fields[2]}
		if dir == "" {
			entry.Path = entry.Name
		} else {
			entry.Path = dir + "/" + entry.Name
		}
		if fields[1] == "tree" {
			entry.IsDir = true
		} else if size, err := strconv.ParseInt(fields[3], 10, 64); err == nil {
			entry.Size = size
		}
		entries = append(entries, entry)
	}
	return entries
}

// runGit chạy git với timeout cố định và trả stdout.
func runGit(repoPath string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	full := append([]string{"--git-dir", repoPath}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("git quá thời gian chờ")
		}
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Package repoinfo đọc thông tin nhánh, tag và commit của một bare repo.
package repoinfo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// commandTimeout giới hạn thời gian mỗi lệnh git đọc thông tin.
const commandTimeout = 5 * time.Second

// DefaultCommitLimit là số commit tối đa hiển thị trên trang chi tiết.
const DefaultCommitLimit = 20

// Commit là một dòng trong lịch sử commit.
type Commit struct {
	Hash    string
	Author  string
	Date    time.Time
	Subject string
}

// Info tổng hợp thông tin hiển thị của một repo.
type Info struct {
	DefaultBranch string
	Branches      []string
	Tags          []string
	Commits       []Commit
	// Empty cho biết repo chưa có commit nào.
	Empty bool
	// Err giữ lỗi đọc thông tin để tầng web hiển thị thay vì làm hỏng trang.
	Err error
}

// Read thu thập thông tin của bare repo tại path.
func Read(path string, limit int) Info {
	if limit <= 0 {
		limit = DefaultCommitLimit
	}
	info := Info{DefaultBranch: "main"}

	branches, err := lines(path, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		info.Err = err
		return info
	}
	tags, err := lines(path, "for-each-ref", "--format=%(refname:short)", "refs/tags")
	if err != nil {
		info.Err = err
		return info
	}
	info.Branches = branches
	info.Tags = tags

	if branch, ok := resolveDefaultBranch(path, branches); ok {
		info.DefaultBranch = branch
	}

	commits, err := readCommits(path, limit)
	if err != nil {
		info.Err = err
		return info
	}
	info.Commits = commits
	info.Empty = len(commits) == 0 && len(branches) == 0 && len(tags) == 0
	return info
}

// LastCommit trả commit mới nhất của repo, dùng cho trang danh sách.
func LastCommit(path string) (Commit, bool) {
	commits, err := readCommits(path, 1)
	if err != nil || len(commits) == 0 {
		return Commit{}, false
	}
	return commits[0], true
}

// Branch trả tên nhánh mặc định của repo, false nếu không đọc được.
func Branch(path string) (string, bool) {
	branches, err := lines(path, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return "", false
	}
	return resolveDefaultBranch(path, branches)
}

// resolveDefaultBranch ưu tiên HEAD, sau đó tới nhánh có thật đầu tiên.
func resolveDefaultBranch(path string, branches []string) (string, bool) {
	if out, err := output(path, "symbolic-ref", "--short", "HEAD"); err == nil {
		if name := strings.TrimSpace(out); name != "" {
			if len(branches) == 0 || contains(branches, name) {
				return name, true
			}
		}
	}
	if len(branches) > 0 {
		return branches[0], true
	}
	return "", false
}

// contains cho biết danh sách có chứa giá trị cần tìm.
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// readCommits đọc tối đa limit commit trên mọi nhánh và tag.
func readCommits(path string, limit int) ([]Commit, error) {
	out, err := output(path, "log", "--all", "-n", strconv.Itoa(limit), "--date=iso-strict",
		"--pretty=format:%h%x1f%an%x1f%ad%x1f%s")
	if err != nil {
		// Repo rỗng khiến git log thoát lỗi, coi như không có commit.
		if isEmptyRepo(path) {
			return nil, nil
		}
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	var commits []Commit
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "\x1f", 4)
		if len(parts) != 4 {
			continue
		}
		commit := Commit{Hash: parts[0], Author: parts[1], Subject: parts[3]}
		if date, err := time.Parse(time.RFC3339, parts[2]); err == nil {
			commit.Date = date
		}
		commits = append(commits, commit)
	}
	return commits, nil
}

// isEmptyRepo cho biết repo chưa có tham chiếu nào.
func isEmptyRepo(path string) bool {
	out, err := output(path, "for-each-ref", "--count=1", "--format=%(refname)")
	return err == nil && strings.TrimSpace(out) == ""
}

// lines chạy git và trả các dòng khác rỗng.
func lines(path string, args ...string) ([]string, error) {
	out, err := output(path, args...)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

// output chạy git với timeout cố định và trả stdout.
func output(path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	full := append([]string{"--git-dir", path}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errors.New("git quá thời gian chờ")
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("git %s: %w: %s", args[0], err, msg)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return string(out), nil
}

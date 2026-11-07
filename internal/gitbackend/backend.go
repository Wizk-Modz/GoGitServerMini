// Package gitbackend chuyển tiếp giao thức Git Smart HTTP sang git http-backend.
package gitbackend

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"gogitservermini/internal/repostore"
)

// Handler phục vụ các request Git qua CGI của git http-backend.
type Handler struct {
	store   *repostore.Store
	maxBody int64
	logger  *slog.Logger
	gitPath string
}

// New tạo Handler cho các repo trong store.
func New(store *repostore.Store, maxBody int64, logger *slog.Logger) *Handler {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		gitPath = "git"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{store: store, maxBody: maxBody, logger: logger, gitPath: gitPath}
}

// ServeHTTP xử lý mọi đường dẫn dạng /<repo>.git/<phần còn lại>.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name, pathInfo, ok := splitRepoPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !h.store.Exists(name) {
		http.NotFound(w, r)
		return
	}
	if r.ContentLength > 0 && h.maxBody > 0 && r.ContentLength > h.maxBody {
		http.Error(w, "dữ liệu gửi lên quá lớn", http.StatusRequestEntityTooLarge)
		return
	}
	// git http-backend cần CONTENT_LENGTH để đọc đúng phần body của giao thức.
	if r.ContentLength < 0 && hasRequestBody(r.Method) {
		http.Error(w, "cần có Content-Length cho request có body", http.StatusLengthRequired)
		return
	}
	body := r.Body
	if h.maxBody > 0 {
		body = http.MaxBytesReader(w, r.Body, h.maxBody)
	}
	defer body.Close()

	ctx := r.Context()
	cmd := exec.CommandContext(ctx, h.gitPath, "http-backend")
	cmd.Env = h.cgiEnv(r, pathInfo)
	cmd.Stdin = body
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		h.logger.Error("không mở được stdout của git http-backend", "err", err)
		http.Error(w, "lỗi máy chủ", http.StatusInternalServerError)
		return
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		h.logger.Error("không chạy được git http-backend", "err", err)
		http.Error(w, "lỗi máy chủ", http.StatusInternalServerError)
		return
	}

	written, writeErr := h.copyCGIResponse(w, stdout)
	if err := cmd.Wait(); err != nil {
		h.logger.Error("git http-backend thoát lỗi", "path", pathInfo, "err", err, "stderr", strings.TrimSpace(stderr.String()))
	} else if strings.HasSuffix(pathInfo, "/git-receive-pack") {
		if err := h.store.FixHead(name); err != nil {
			h.logger.Warn("không cập nhật được HEAD sau push", "name", name, "err", err)
		}
	}
	if writeErr != nil {
		h.logger.Warn("không ghi hết phản hồi git", "path", pathInfo, "err", writeErr)
		if !written {
			http.Error(w, "lỗi máy chủ", http.StatusInternalServerError)
		}
	}
}

// copyCGIResponse đọc header CGI rồi stream phần body ra client.
func (h *Handler) copyCGIResponse(w http.ResponseWriter, stdout io.Reader) (bool, error) {
	reader := bufio.NewReader(stdout)
	status := http.StatusOK
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && line == "" {
				return false, fmt.Errorf("git http-backend không trả về header")
			}
			return false, fmt.Errorf("đọc header CGI: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if strings.EqualFold(key, "Status") {
			if code, _, ok := strings.Cut(value, " "); ok {
				if parsed, err := strconv.Atoi(code); err == nil {
					status = parsed
				}
			}
			continue
		}
		w.Header().Add(key, value)
	}
	w.WriteHeader(status)
	_, err := io.Copy(w, reader)
	return true, err
}

// cgiEnv dựng biến môi trường CGI cho git http-backend.
func (h *Handler) cgiEnv(r *http.Request, pathInfo string) []string {
	env := []string{
		"GIT_PROJECT_ROOT=" + h.store.Root,
		"GIT_HTTP_EXPORT_ALL=1",
		"PATH_INFO=" + pathInfo,
		"REQUEST_METHOD=" + r.Method,
		"QUERY_STRING=" + r.URL.RawQuery,
		"REMOTE_ADDR=" + remoteAddr(r),
		"REMOTE_USER=",
		"GATEWAY_INTERFACE=CGI/1.1",
		"SERVER_PROTOCOL=HTTP/1.1",
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		env = append(env, "CONTENT_TYPE="+ct)
	}
	if r.ContentLength >= 0 {
		env = append(env, "CONTENT_LENGTH="+strconv.FormatInt(r.ContentLength, 10))
	}
	for key, values := range r.Header {
		name := strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
		if name == "CONTENT_TYPE" || name == "CONTENT_LENGTH" {
			continue
		}
		env = append(env, "HTTP_"+name+"="+strings.Join(values, ", "))
	}
	return env
}

// splitRepoPath tách tên repo và PATH_INFO từ đường dẫn yêu cầu.
func splitRepoPath(urlPath string) (string, string, bool) {
	name, ok := RepoName(urlPath)
	if !ok {
		return "", "", false
	}
	return name, urlPath, true
}

// RepoName lấy tên repo từ đường dẫn dạng /<name>.git/<phần còn lại>.
func RepoName(urlPath string) (string, bool) {
	trimmed := strings.TrimPrefix(urlPath, "/")
	slash := strings.IndexByte(trimmed, '/')
	if slash <= 0 {
		return "", false
	}
	name, ok := strings.CutSuffix(trimmed[:slash], ".git")
	if !ok || repostore.ValidateName(name) != nil {
		return "", false
	}
	return name, true
}

// remoteAddr trả địa chỉ IP của client, bỏ phần port.
func remoteAddr(r *http.Request) string {
	host := r.RemoteAddr
	if idx := strings.LastIndexByte(host, ':'); idx > 0 {
		return host[:idx]
	}
	return host
}

// hasRequestBody cho biết method có gửi body theo giao thức git.
func hasRequestBody(method string) bool {
	return method == http.MethodPost
}

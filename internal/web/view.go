package web

import (
	"bytes"
	"html/template"
	"net/http"
	"strconv"
	"time"
)

// repoRow là một mục trong danh sách repo ở trang chủ.
type repoRow struct {
	Name        string
	Description string
	Branch      string
	Subject     string
	Author      string
	When        string
	Empty       bool
	HasCommit   bool
}

// indexData là dữ liệu cho trang chủ.
type indexData struct {
	Title       string
	Root        string
	Meta        string
	BaseURL     string
	Error       string
	Name        string
	Repos       []repoRow
	TotalRepos  int
	ActiveRepos int
}

// commitRow là một commit đã định dạng để hiển thị.
type commitRow struct {
	Hash    string
	Author  string
	Subject string
	When    string
}

// repoData là dữ liệu cho trang chi tiết repo.
type repoData struct {
	Title         string
	Name          string
	Description   string
	CreatedAt     string
	UpdatedAt     string
	CloneURL      string
	DefaultBranch string
	Error         string
	Empty         bool
	Branches      []string
	Tags          []string
	Commits       []commitRow
}

// renderPage ghi một trang HTML hoàn chỉnh với status cho trước.
func (h *Handler) renderPage(w http.ResponseWriter, tmpl *template.Template, data any, status int) {
	var buf bytes.Buffer
	if err := render(&buf, tmpl, data); err != nil {
		h.logger.Error("không render được trang", "err", err)
		http.Error(w, "lỗi máy chủ", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// renderNotFound hiển thị trang lỗi HTML với mã trạng thái tương ứng.
func (h *Handler) renderNotFound(w http.ResponseWriter, status int, message string) {
	heading := "Không tìm thấy trang"
	switch status {
	case http.StatusInternalServerError:
		heading = "Máy chủ gặp sự cố"
	case http.StatusBadRequest:
		heading = "Yêu cầu không hợp lệ"
	}
	data := struct {
		Title   string
		Code    string
		Heading string
		Message string
	}{Title: heading, Code: strconv.Itoa(status), Heading: heading, Message: message}
	h.renderPage(w, h.notFoundTmpl, data, status)
}

// baseURL dựng URL gốc của server từ request hiện tại.
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// formatTime định dạng thời gian commit theo giờ máy chủ.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04")
}

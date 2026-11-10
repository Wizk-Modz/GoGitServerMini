// Package web cung cấp giao diện HTML để quản lý repo.
package web

import (
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"gogitservermini/internal/gitbackend"
	"gogitservermini/internal/metastore"
	"gogitservermini/internal/repoinfo"
	"gogitservermini/internal/repostore"
)

// maxFormBytes giới hạn kích thước form tạo repo.
const maxFormBytes = 8 << 10

// maxDescriptionBytes giới hạn độ dài mô tả repo.
const maxDescriptionBytes = 500

// Handler phục vụ toàn bộ route web và route git.
type Handler struct {
	store  *repostore.Store
	meta   *metastore.Store
	git    *gitbackend.Handler
	logger *slog.Logger

	indexTmpl    *template.Template
	repoTmpl     *template.Template
	notFoundTmpl *template.Template
}

// New dựng Handler hoàn chỉnh với các route đã đăng ký.
func New(store *repostore.Store, meta *metastore.Store, maxBody int64, logger *slog.Logger) (*Handler, error) {
	if logger == nil {
		logger = slog.Default()
	}
	indexTmpl, err := newPageTemplate("index.html")
	if err != nil {
		return nil, err
	}
	repoTmpl, err := newPageTemplate("repo.html")
	if err != nil {
		return nil, err
	}
	notFoundTmpl, err := newPageTemplate("notfound.html")
	if err != nil {
		return nil, err
	}
	return &Handler{
		store:        store,
		meta:         meta,
		git:          gitbackend.New(store, maxBody, logger),
		logger:       logger,
		indexTmpl:    indexTmpl,
		repoTmpl:     repoTmpl,
		notFoundTmpl: notFoundTmpl,
	}, nil
}

// Routes trả mux đã gắn đầy đủ handler.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.handleIndex)
	mux.HandleFunc("POST /new", h.handleCreate)
	mux.HandleFunc("GET /repo/{name}", h.handleRepo)
	mux.HandleFunc("POST /repo/{name}/edit", h.handleEdit)
	mux.HandleFunc("POST /repo/{name}/delete", h.handleDelete)
	mux.HandleFunc("/", h.handleFallback)
	return mux
}

// handleIndex hiển thị danh sách repo.
func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	h.renderIndex(w, r, http.StatusOK, "", "")
}

// renderIndex dựng dữ liệu danh sách repo rồi ghi trang chủ.
func (h *Handler) renderIndex(w http.ResponseWriter, r *http.Request, status int, errMsg, name string) {
	names, err := h.store.List()
	if err != nil {
		h.logger.Error("không đọc được danh sách repo", "err", err)
		h.renderNotFound(w, http.StatusInternalServerError, "Không đọc được danh sách repo.")
		return
	}
	rows := make([]repoRow, 0, len(names))
	for _, repoName := range names {
		row := repoRow{Name: repoName}
		if repo, ok := h.meta.Get(repoName); ok {
			row.Description = repo.Description
		}
		if path, err := h.store.Path(repoName); err == nil {
			if branch, ok := repoinfo.Branch(path); ok {
				row.Branch = branch
			}
			if commit, ok := repoinfo.LastCommit(path); ok {
				row.HasCommit = true
				row.Subject = commit.Subject
				row.Author = commit.Author
				row.When = formatTime(commit.Date)
			}
		}
		row.Empty = !row.HasCommit
		rows = append(rows, row)
	}

	activeRepos := 0
	for _, row := range rows {
		if row.HasCommit {
			activeRepos++
		}
	}
	data := indexData{
		Title:       "Kho lưu trữ",
		Root:        h.store.Root,
		Meta:        h.meta.Path(),
		BaseURL:     baseURL(r),
		Error:       errMsg,
		Name:        name,
		Repos:       rows,
		TotalRepos:  len(rows),
		ActiveRepos: activeRepos,
	}
	h.renderPage(w, h.indexTmpl, data, status)
}

// handleCreate tạo repo mới từ form.
func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderIndex(w, r, http.StatusBadRequest, "Không đọc được dữ liệu form.", "")
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		h.renderIndex(w, r, http.StatusBadRequest, "Vui lòng nhập tên repo.", "")
		return
	}
	if err := repostore.ValidateName(name); err != nil {
		h.renderIndex(w, r, http.StatusBadRequest,
			"Tên repo không hợp lệ. Chỉ dùng chữ, số, dấu chấm, gạch ngang, gạch dưới và không bắt đầu bằng dấu chấm.", name)
		return
	}
	description, errMsg := normalizeDescription(r.PostFormValue("description"))
	if errMsg != "" {
		h.renderIndex(w, r, http.StatusBadRequest, errMsg, name)
		return
	}
	switch err := h.store.Create(name); {
	case errors.Is(err, repostore.ErrExists):
		h.renderIndex(w, r, http.StatusConflict, "Repo \""+name+"\" đã tồn tại.", name)
		return
	case err != nil:
		h.logger.Error("không tạo được repo", "name", name, "err", err)
		h.renderIndex(w, r, http.StatusInternalServerError, "Không tạo được repo. Xem log máy chủ.", name)
		return
	}
	if description != "" {
		if err := h.meta.Set(name, description); err != nil {
			h.logger.Error("không lưu được metadata repo", "name", name, "err", err)
		}
	}
	http.Redirect(w, r, "/repo/"+name, http.StatusSeeOther)
}

// handleEdit cập nhật mô tả của repo.
func (h *Handler) handleEdit(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !h.store.Exists(name) {
		h.renderNotFound(w, http.StatusNotFound, "Repo \""+name+"\" không tồn tại.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		h.renderRepo(w, r, name, http.StatusBadRequest, "Không đọc được dữ liệu form.")
		return
	}
	description, errMsg := normalizeDescription(r.PostFormValue("description"))
	if errMsg != "" {
		h.renderRepo(w, r, name, http.StatusBadRequest, errMsg)
		return
	}
	if err := h.meta.Set(name, description); err != nil {
		h.logger.Error("không lưu được metadata repo", "name", name, "err", err)
		h.renderRepo(w, r, name, http.StatusInternalServerError, "Không lưu được mô tả. Xem log máy chủ.")
		return
	}
	http.Redirect(w, r, "/repo/"+name, http.StatusSeeOther)
}

// handleRepo hiển thị chi tiết một repo.
func (h *Handler) handleRepo(w http.ResponseWriter, r *http.Request) {
	h.renderRepo(w, r, r.PathValue("name"), http.StatusOK, "")
}

// renderRepo dựng dữ liệu chi tiết repo rồi ghi trang.
func (h *Handler) renderRepo(w http.ResponseWriter, r *http.Request, name string, status int, errMsg string) {
	path, err := h.store.Path(name)
	if err != nil {
		h.renderNotFound(w, http.StatusNotFound, "Tên repo không hợp lệ.")
		return
	}
	if !h.store.Exists(name) {
		h.renderNotFound(w, http.StatusNotFound, "Repo \""+name+"\" không tồn tại.")
		return
	}

	info := repoinfo.Read(path, repoinfo.DefaultCommitLimit)
	data := repoData{
		Title:         name,
		Name:          name,
		CloneURL:      baseURL(r) + "/" + name + ".git",
		DefaultBranch: info.DefaultBranch,
		Error:         errMsg,
		Empty:         info.Empty,
		Branches:      info.Branches,
		Tags:          info.Tags,
	}
	if repo, ok := h.meta.Get(name); ok {
		data.Description = repo.Description
		data.CreatedAt = formatTime(repo.CreatedAt)
		data.UpdatedAt = formatTime(repo.UpdatedAt)
	}
	if info.Err != nil {
		h.logger.Warn("không đọc được thông tin repo", "name", name, "err", info.Err)
		data.Error = "Không đọc được đầy đủ thông tin repo."
	}
	for _, commit := range info.Commits {
		data.Commits = append(data.Commits, commitRow{
			Hash:    commit.Hash,
			Author:  commit.Author,
			Subject: commit.Subject,
			When:    formatTime(commit.Date),
		})
	}
	h.renderPage(w, h.repoTmpl, data, status)
}

// normalizeDescription cắt khoảng trắng và kiểm tra độ dài mô tả.
func normalizeDescription(raw string) (string, string) {
	description := strings.TrimSpace(raw)
	if len(description) > maxDescriptionBytes {
		return "", "Mô tả quá dài, tối đa 500 ký tự."
	}
	return description, ""
}

// handleDelete xoá repo sau khi người dùng xác nhận trên form.
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.store.Delete(name); err != nil {
		switch {
		case errors.Is(err, repostore.ErrNotFound):
			h.renderNotFound(w, http.StatusNotFound, "Repo \""+name+"\" không tồn tại.")
		case errors.Is(err, repostore.ErrInvalidName):
			h.renderNotFound(w, http.StatusNotFound, "Tên repo không hợp lệ.")
		default:
			h.logger.Error("không xoá được repo", "name", name, "err", err)
			h.renderNotFound(w, http.StatusInternalServerError, "Không xoá được repo. Xem log máy chủ.")
		}
		return
	}
	h.logger.Info("đã xoá repo", "name", name)
	if err := h.meta.Delete(name); err != nil {
		h.logger.Error("không xoá được metadata repo", "name", name, "err", err)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleFallback chuyển request git cho git backend, còn lại trả 404 HTML.
func (h *Handler) handleFallback(w http.ResponseWriter, r *http.Request) {
	if name, ok := gitbackend.RepoName(r.URL.Path); ok {
		if !h.store.Exists(name) {
			h.renderNotFound(w, http.StatusNotFound, "Repo \""+name+"\" không tồn tại.")
			return
		}
		h.git.ServeHTTP(w, r)
		return
	}
	h.renderNotFound(w, http.StatusNotFound, "Đường dẫn không tồn tại.")
}

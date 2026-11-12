package web

import (
	"errors"
	"html/template"
	"io"
	"net/http"
	"path"
	"strings"

	"gogitservermini/internal/markdown"
	"gogitservermini/internal/repofiles"
	"gogitservermini/internal/repoinfo"
)

// maxCodeLines giới hạn số dòng hiển thị cho tệp văn bản dài.
const maxCodeLines = 5000

// maxRepoFiles giới hạn số mục tệp hiển thị trên trang chi tiết repo.
const maxRepoFiles = 12

// breadcrumb là một mắt trong thanh điều hướng đường dẫn.
type breadcrumb struct {
	Label string
	URL   string
}

// treeEntryRow là một mục tệp hoặc thư mục để hiển thị.
type treeEntryRow struct {
	Name  string
	Path  string
	URL   string
	Size  string
	IsDir bool
}

// treeData là dữ liệu cho trang duyệt thư mục.
type treeData struct {
	Title       string
	Name        string
	Ref         string
	Path        string
	Entries     []treeEntryRow
	Breadcrumbs []breadcrumb
	ParentURL   string
}

// codeLine là một dòng văn bản trong khung xem code.
type codeLine struct {
	Text string
}

// blobData là dữ liệu cho trang xem tệp.
type blobData struct {
	Title       string
	Name        string
	Ref         string
	Path        string
	Size        string
	Breadcrumbs []breadcrumb
	RawURL      string
	BlobURL     string
	IsMarkdown  bool
	HTML        template.HTML
	Lines       []codeLine
	Binary      bool
	TooLarge    bool
	Truncated   bool
	ImageURL    string
	ImageType   string
}

// handleTree hiển thị danh sách tệp trong một thư mục của repo.
func (h *Handler) handleTree(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	repoPath, ok := h.repoPath(w, name)
	if !ok {
		return
	}
	ref, ok := h.resolveRef(w, r, repoPath, name)
	if !ok {
		return
	}
	dir := strings.TrimSuffix(r.PathValue("path"), "/")
	kind, err := repofiles.Type(repoPath, ref, dir)
	if err != nil {
		h.renderFileError(w, r, name, ref, dir, err)
		return
	}
	if kind == "blob" {
		http.Redirect(w, r, blobURL(name, dir, ref), http.StatusSeeOther)
		return
	}
	entries, err := repofiles.Tree(repoPath, ref, dir)
	if err != nil {
		h.renderFileError(w, r, name, ref, dir, err)
		return
	}
	data := treeData{
		Title:       name + " · " + dirLabel(dir),
		Name:        name,
		Ref:         ref,
		Path:        dir,
		Entries:     h.treeRows(name, ref, entries),
		Breadcrumbs: buildBreadcrumbs(name, ref, dir, false),
		ParentURL:   parentTreeURL(name, ref, dir),
	}
	h.renderPage(w, h.treeTmpl, data, http.StatusOK)
}

// handleBlob hiển thị nội dung một tệp, ảnh hoặc chuyển hướng thư mục.
func (h *Handler) handleBlob(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	repoPath, ok := h.repoPath(w, name)
	if !ok {
		return
	}
	ref, ok := h.resolveRef(w, r, repoPath, name)
	if !ok {
		return
	}
	filePath := strings.TrimSuffix(r.PathValue("path"), "/")
	if filePath == "" {
		http.Redirect(w, r, treeURL(name, "", ref), http.StatusSeeOther)
		return
	}
	kind, err := repofiles.Type(repoPath, ref, filePath)
	if err != nil {
		h.renderFileError(w, r, name, ref, filePath, err)
		return
	}
	if kind == "tree" {
		http.Redirect(w, r, treeURL(name, filePath, ref), http.StatusSeeOther)
		return
	}

	blob, err := repofiles.Read(repoPath, ref, filePath)
	if errors.Is(err, repofiles.ErrTooLarge) {
		h.renderBlobNotice(w, r, name, ref, filePath, blob.Size, false)
		return
	}
	if err != nil {
		h.renderFileError(w, r, name, ref, filePath, err)
		return
	}
	if blob.Binary {
		if mediaType, ok := inlineImageType(filePath); ok {
			h.renderImage(w, r, name, ref, filePath, blob.Size, mediaType)
			return
		}
		h.renderBlobNotice(w, r, name, ref, filePath, blob.Size, true)
		return
	}

	data := blobData{
		Title:       filePath + " · " + name,
		Name:        name,
		Ref:         ref,
		Path:        filePath,
		Size:        formatSize(blob.Size),
		Breadcrumbs: buildBreadcrumbs(name, ref, filePath, true),
		RawURL:      rawURL(name, filePath, ref),
		BlobURL:     blobURL(name, filePath, ref),
	}
	if isMarkdownPath(filePath) {
		data.IsMarkdown = true
		opts := markdownOptions(name, ref, path.Dir(filePath))
		data.HTML = template.HTML(markdown.RenderWith(blob.Data, opts))
	} else {
		lines := strings.Split(string(blob.Data), "\n")
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		if len(lines) > maxCodeLines {
			lines = lines[:maxCodeLines]
			data.Truncated = true
		}
		data.Lines = make([]codeLine, len(lines))
		for i, line := range lines {
			data.Lines[i] = codeLine{Text: line}
		}
	}
	h.renderPage(w, h.blobTmpl, data, http.StatusOK)
}

// handleRaw trả nội dung thô của tệp để tải hoặc nhúng, không giới hạn kích thước.
func (h *Handler) handleRaw(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	repoPath, ok := h.repoPath(w, name)
	if !ok {
		return
	}
	ref, ok := h.resolveRef(w, r, repoPath, name)
	if !ok {
		return
	}
	filePath := strings.TrimSuffix(r.PathValue("path"), "/")
	if filePath == "" {
		h.renderNotFound(w, http.StatusNotFound, "Đường dẫn tệp không hợp lệ.")
		return
	}
	kind, err := repofiles.Type(repoPath, ref, filePath)
	if err != nil {
		h.renderFileError(w, r, name, ref, filePath, err)
		return
	}
	if kind != "blob" {
		h.renderFileError(w, r, name, ref, filePath, repofiles.ErrNotFound)
		return
	}
	stream, err := repofiles.Reader(repoPath, ref, filePath)
	if err != nil {
		h.renderFileError(w, r, name, ref, filePath, err)
		return
	}
	defer stream.Close()

	contentType := "text/plain; charset=utf-8"
	if mediaType, ok := inlineImageType(filePath); ok {
		contentType = mediaType
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("Content-Disposition", "inline; filename=\""+sanitizeFilename(path.Base(filePath))+"\"")
	if _, err := io.Copy(w, stream); err != nil {
		h.logger.Warn("không gửi hết nội dung tệp", "name", name, "path", filePath, "err", err)
	}
}

// repoPath trả đường dẫn bare repo và ghi lỗi 404 nếu không hợp lệ.
func (h *Handler) repoPath(w http.ResponseWriter, name string) (string, bool) {
	repoPath, err := h.store.Path(name)
	if err != nil || !h.store.Exists(name) {
		h.renderNotFound(w, http.StatusNotFound, "Repo \""+name+"\" không tồn tại.")
		return "", false
	}
	return repoPath, true
}

// resolveRef chọn ref từ query, mặc định là nhánh mặc định của repo.
func (h *Handler) resolveRef(w http.ResponseWriter, r *http.Request, repoPath, name string) (string, bool) {
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if ref != "" {
		return ref, true
	}
	if branch, ok := h.defaultBranch(repoPath); ok {
		return branch, true
	}
	h.renderNotFound(w, http.StatusNotFound, "Repo \""+name+"\" chưa có nhánh nào để xem.")
	return "", false
}

// defaultBranch trả nhánh mặc định của repo, false nếu repo trống.
func (h *Handler) defaultBranch(repoPath string) (string, bool) {
	return repoinfo.Branch(repoPath)
}

// treeRows chuyển các mục git thành dữ liệu hiển thị, thư mục đứng trước.
func (h *Handler) treeRows(name, ref string, entries []repofiles.Entry) []treeEntryRow {
	dirs := make([]treeEntryRow, 0, len(entries))
	files := make([]treeEntryRow, 0, len(entries))
	for _, entry := range entries {
		row := treeEntryRow{Name: entry.Name, Path: entry.Path, IsDir: entry.IsDir}
		if entry.IsDir {
			row.URL = treeURL(name, entry.Path, ref)
			dirs = append(dirs, row)
			continue
		}
		row.URL = blobURL(name, entry.Path, ref)
		row.Size = formatSize(entry.Size)
		files = append(files, row)
	}
	return append(dirs, files...)
}

// renderBlobNotice hiển thị trang thông báo cho tệp không xem trực tiếp được.
func (h *Handler) renderBlobNotice(w http.ResponseWriter, r *http.Request, name, ref, filePath string, size int64, binary bool) {
	data := blobData{
		Title:       filePath + " · " + name,
		Name:        name,
		Ref:         ref,
		Path:        filePath,
		Size:        formatSize(size),
		Breadcrumbs: buildBreadcrumbs(name, ref, filePath, true),
		RawURL:      rawURL(name, filePath, ref),
		BlobURL:     blobURL(name, filePath, ref),
		Binary:      binary,
		TooLarge:    !binary,
	}
	h.renderPage(w, h.blobTmpl, data, http.StatusOK)
}

// renderImage hiển thị ảnh trong khung xem kèm điều hướng thay vì trả bytes trần.
func (h *Handler) renderImage(w http.ResponseWriter, r *http.Request, name, ref, filePath string, size int64, mediaType string) {
	data := blobData{
		Title:       filePath + " · " + name,
		Name:        name,
		Ref:         ref,
		Path:        filePath,
		Size:        formatSize(size),
		Breadcrumbs: buildBreadcrumbs(name, ref, filePath, true),
		RawURL:      rawURL(name, filePath, ref),
		BlobURL:     blobURL(name, filePath, ref),
		ImageURL:    rawURL(name, filePath, ref),
		ImageType:   mediaType,
	}
	h.renderPage(w, h.blobTmpl, data, http.StatusOK)
}

// renderFileError chuyển lỗi đọc tệp thành trang lỗi phù hợp.
func (h *Handler) renderFileError(w http.ResponseWriter, r *http.Request, name, ref, filePath string, err error) {
	switch {
	case errors.Is(err, repofiles.ErrInvalidPath):
		h.renderNotFound(w, http.StatusBadRequest, "Đường dẫn tệp không hợp lệ.")
	case errors.Is(err, repofiles.ErrNotFound):
		message := "Không tìm thấy tệp hoặc thư mục \"" + filePath + "\"."
		if ref != "" {
			message = "Không tìm thấy tệp hoặc thư mục \"" + filePath + "\" trên ref \"" + ref + "\"."
		}
		h.renderNotFound(w, http.StatusNotFound, message)
	default:
		h.logger.Error("không đọc được tệp repo", "name", name, "path", filePath, "err", err)
		h.renderNotFound(w, http.StatusInternalServerError, "Không đọc được tệp. Xem log máy chủ.")
	}
}

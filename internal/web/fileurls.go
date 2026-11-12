package web

import (
	"mime"
	"net/url"
	"path"
	"strconv"
	"strings"

	"gogitservermini/internal/markdown"
)

// buildBreadcrumbs dựng danh sách đường dẫn từ tên repo tới tệp hiện tại.
func buildBreadcrumbs(name, ref, filePath string, isFile bool) []breadcrumb {
	items := []breadcrumb{{Label: name, URL: "/repo/" + url.PathEscape(name)}}
	if filePath == "" {
		return items
	}
	parts := strings.Split(filePath, "/")
	current := ""
	for i, part := range parts {
		if current == "" {
			current = part
		} else {
			current += "/" + part
		}
		last := i == len(parts)-1
		item := breadcrumb{Label: part}
		if !last || !isFile {
			item.URL = treeURL(name, current, ref)
		}
		items = append(items, item)
	}
	return items
}

// treeURL dựng URL trang duyệt thư mục kèm ref.
func treeURL(name, dir, ref string) string {
	return withRef(repoFileURL(name, "tree", dir), ref)
}

// blobURL dựng URL trang xem tệp kèm ref.
func blobURL(name, filePath, ref string) string {
	return withRef(repoFileURL(name, "blob", filePath), ref)
}

// rawURL dựng URL tải nội dung thô kèm ref.
func rawURL(name, filePath, ref string) string {
	return withRef(repoFileURL(name, "raw", filePath), ref)
}

// repoFileURL ghép URL tệp trong repo, escape từng đoạn đường dẫn để tên có ký tự đặc biệt vẫn mở được.
func repoFileURL(name, action, filePath string) string {
	base := "/repo/" + url.PathEscape(name) + "/" + action + "/"
	if filePath == "" {
		return base
	}
	segments := strings.Split(filePath, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return base + strings.Join(segments, "/")
}

// parentTreeURL trả URL thư mục cha, rỗng nếu đang ở gốc.
func parentTreeURL(name, ref, dir string) string {
	if dir == "" {
		return ""
	}
	parent := path.Dir(dir)
	if parent == "." {
		parent = ""
	}
	return treeURL(name, parent, ref)
}

// withRef gắn tham số ref vào URL khi cần.
func withRef(target, ref string) string {
	if ref == "" {
		return target
	}
	return target + "?ref=" + url.QueryEscape(ref)
}

// markdownOptions trả hook đổi link và ảnh tương đối trong Markdown thành URL trong repo.
func markdownOptions(name, ref, baseDir string) markdown.Options {
	return markdown.Options{
		Link: func(dest string) string {
			return resolveMarkdownURL(name, ref, baseDir, dest, false)
		},
		Image: func(dest string) string {
			return resolveMarkdownURL(name, ref, baseDir, dest, true)
		},
	}
}

// resolveMarkdownURL đổi một URL trong Markdown thành URL nội bộ repo, rỗng nếu giữ nguyên.
func resolveMarkdownURL(name, ref, baseDir, dest string, image bool) string {
	if dest == "" {
		return ""
	}
	lower := strings.ToLower(dest)
	if strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "//") ||
		strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:") {
		return ""
	}
	// URL có scheme khác do tầng markdown tự chặn, không đổi thành đường dẫn nội bộ.
	if i := strings.Index(dest, ":"); i > 0 && !strings.Contains(dest[:i], "/") {
		return ""
	}
	target, suffix := splitURLSuffix(dest)
	if target == "" {
		return ""
	}

	isDir := strings.HasSuffix(target, "/")
	var resolved string
	if strings.HasPrefix(target, "/") {
		resolved = path.Clean("/" + target)[1:]
	} else {
		resolved = path.Clean(path.Join(baseDir, target))
	}
	// "." là thư mục gốc của repo, còn ".." nghĩa là thoát khỏi repo nên giữ link gốc.
	if resolved == "." {
		resolved = ""
	}
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return ""
	}

	if image {
		if resolved == "" {
			return ""
		}
		return rawURL(name, resolved, ref)
	}
	if isDir {
		return appendURLSuffix(treeURL(name, resolved, ref), suffix)
	}
	return appendURLSuffix(blobURL(name, resolved, ref), suffix)
}

// splitURLSuffix tách phần query và fragment khỏi đường dẫn.
func splitURLSuffix(dest string) (string, string) {
	if i := strings.IndexAny(dest, "?#"); i >= 0 {
		return dest[:i], dest[i:]
	}
	return dest, ""
}

// appendURLSuffix gắn query hoặc fragment vào URL đã dựng.
func appendURLSuffix(target, suffix string) string {
	if suffix == "" {
		return target
	}
	if strings.HasPrefix(suffix, "#") {
		return target + suffix
	}
	if strings.Contains(target, "?") {
		return target + "&" + strings.TrimPrefix(suffix, "?")
	}
	return target + suffix
}

// dirLabel trả nhãn hiển thị cho thư mục hiện tại.
func dirLabel(dir string) string {
	if dir == "" {
		return "gốc"
	}
	return path.Base(dir)
}

// formatSize định dạng kích thước tệp theo đơn vị dễ đọc.
func formatSize(size int64) string {
	switch {
	case size < 1024:
		return strconv.FormatInt(size, 10) + " B"
	case size < 1024*1024:
		return strconv.FormatFloat(float64(size)/1024, 'f', 1, 64) + " KB"
	default:
		return strconv.FormatFloat(float64(size)/(1024*1024), 'f', 1, 64) + " MB"
	}
}

// isMarkdownPath cho biết tệp có phải Markdown hay không.
func isMarkdownPath(filePath string) bool {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	}
	return false
}

// inlineImageType trả MIME type cho ảnh raster được phép nhúng trực tiếp.
func inlineImageType(filePath string) (string, bool) {
	ext := strings.ToLower(path.Ext(filePath))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		if mediaType := mime.TypeByExtension(ext); mediaType != "" {
			return mediaType, true
		}
	}
	return "", false
}

// sanitizeFilename loại bỏ ký tự có thể phá vỡ header Content-Disposition.
func sanitizeFilename(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r < 0x20 {
			return -1
		}
		return r
	}, name)
}

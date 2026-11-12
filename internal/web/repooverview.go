package web

import (
	"html/template"
	"path"
	"strings"

	"gogitservermini/internal/markdown"
	"gogitservermini/internal/repofiles"
)

// readmeCandidates là thứ tự ưu tiên tìm tệp giới thiệu repo.
var readmeCandidates = []string{"README.md", "README.markdown", "README.mdown", "README.txt", "README"}

// fillRepoFiles nạp danh sách tệp gốc và nội dung README cho trang tổng quan.
func (h *Handler) fillRepoFiles(data *repoData, repoPath, ref string) {
	if ref == "" {
		return
	}
	entries, err := repofiles.Tree(repoPath, ref, "")
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("không đọc được cây tệp repo", "repo", data.Name, "err", err)
		}
		return
	}
	data.HasTree = true
	data.FileURL = treeURL(data.Name, "", ref)

	dirs := make([]treeEntryRow, 0, len(entries))
	files := make([]treeEntryRow, 0, len(entries))
	for _, entry := range entries {
		row := treeEntryRow{Name: entry.Name, Path: entry.Path, IsDir: entry.IsDir}
		if entry.IsDir {
			row.URL = treeURL(data.Name, entry.Path, ref)
			dirs = append(dirs, row)
			continue
		}
		row.URL = blobURL(data.Name, entry.Path, ref)
		row.Size = formatSize(entry.Size)
		files = append(files, row)
	}
	ordered := append(dirs, files...)
	if len(ordered) > maxRepoFiles {
		ordered = ordered[:maxRepoFiles]
		data.FilesTruncated = true
	}
	data.Files = ordered

	readmeName := findReadme(entries)
	if readmeName == "" {
		return
	}
	blob, err := repofiles.Read(repoPath, ref, readmeName)
	if err != nil || blob.Binary {
		return
	}
	data.ReadmeName = readmeName
	data.ReadmeURL = blobURL(data.Name, readmeName, ref)
	if isMarkdownPath(readmeName) {
		opts := markdownOptions(data.Name, ref, path.Dir(readmeName))
		data.ReadmeHTML = template.HTML(markdown.RenderWith(blob.Data, opts))
		return
	}
	var b strings.Builder
	b.WriteString("<pre><code>")
	b.WriteString(template.HTMLEscapeString(string(blob.Data)))
	b.WriteString("</code></pre>")
	data.ReadmeHTML = template.HTML(b.String())
}

// findReadme trả tên tệp README ưu tiên cao nhất có trong danh sách mục.
func findReadme(entries []repofiles.Entry) string {
	for _, candidate := range readmeCandidates {
		for _, entry := range entries {
			if !entry.IsDir && strings.EqualFold(entry.Name, candidate) {
				return entry.Name
			}
		}
	}
	for _, entry := range entries {
		if !entry.IsDir && strings.HasPrefix(strings.ToUpper(entry.Name), "README") {
			return entry.Name
		}
	}
	return ""
}

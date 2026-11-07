package web

import (
	"embed"
	"html/template"
	"io"
)

//go:embed templates/*.html
var templateFS embed.FS

// newPageTemplate nạp layout chung cùng một trang cụ thể.
func newPageTemplate(page string) (*template.Template, error) {
	return template.ParseFS(templateFS, "templates/layout.html", "templates/"+page)
}

// render ghi trang HTML hoàn chỉnh ra w.
func render(w io.Writer, tmpl *template.Template, data any) error {
	return tmpl.ExecuteTemplate(w, "layout", data)
}

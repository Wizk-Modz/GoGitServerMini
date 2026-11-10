package web

import (
	"embed"
	"fmt"
	"html/template"
	"io"
)

//go:embed templates/*.html templates/*.css
var templateFS embed.FS

// newPageTemplate nạp layout chung cùng một trang cụ thể.
func newPageTemplate(page string) (*template.Template, error) {
	funcs := template.FuncMap{
		"css": func(name string) (template.CSS, error) {
			data, err := templateFS.ReadFile("templates/" + name)
			if err != nil {
				return "", fmt.Errorf("không đọc được CSS %s: %w", name, err)
			}
			return template.CSS(data), nil
		},
	}
	return template.New(page).Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/"+page)
}

// render ghi trang HTML hoàn chỉnh ra w.
func render(w io.Writer, tmpl *template.Template, data any) error {
	return tmpl.ExecuteTemplate(w, "layout", data)
}

package markdown

import (
	"html"
	"net/url"
	"strings"
)

// inline chuyển định dạng nội tuyến sang HTML.
func (r *renderer) inline(text string) string {
	var out strings.Builder
	r.inlineInto(&out, text)
	return out.String()
}

// renderInlineInto quét chuỗi và ghi HTML đã escape ra out.
func (r *renderer) inlineInto(out *strings.Builder, text string) {
	for i := 0; i < len(text); {
		switch text[i] {
		case '\\':
			if i+1 < len(text) && isPunctuation(text[i+1]) {
				out.WriteString(html.EscapeString(text[i+1 : i+2]))
				i += 2
				continue
			}
			out.WriteByte('\\')
			i++
		case '`':
			if rendered, next, ok := parseCodeSpan(text, i); ok {
				out.WriteString(rendered)
				i = next
				continue
			}
			out.WriteString("`")
			i++
		case '!':
			if i+1 < len(text) && text[i+1] == '[' {
				if rendered, next, ok := r.parseImage(text, i); ok {
					out.WriteString(rendered)
					i = next
					continue
				}
			}
			out.WriteByte('!')
			i++
		case '[':
			if rendered, next, ok := r.parseLink(text, i); ok {
				out.WriteString(rendered)
				i = next
				continue
			}
			out.WriteString("[")
			i++
		case '<':
			if rendered, next, ok := parseAutolink(text, i); ok {
				out.WriteString(rendered)
				i = next
				continue
			}
			out.WriteString("&lt;")
			i++
		case '*', '_', '~':
			if rendered, next, ok := r.parseEmphasis(text, i); ok {
				out.WriteString(rendered)
				i = next
				continue
			}
			out.WriteString(html.EscapeString(text[i : i+1]))
			i++
		case '\n':
			if i >= 2 && text[i-1] == ' ' && text[i-2] == ' ' {
				out.WriteString("<br>\n")
			} else {
				out.WriteByte('\n')
			}
			i++
		default:
			start := i
			for i < len(text) && !isInlineSpecial(text[i]) {
				i++
			}
			out.WriteString(html.EscapeString(text[start:i]))
		}
	}
}

// isInlineSpecial cho biết ký tự cần xử lý riêng.
func isInlineSpecial(c byte) bool {
	switch c {
	case '\\', '`', '!', '[', '<', '*', '_', '~', '\n':
		return true
	}
	return false
}

// isPunctuation cho biết ký tự có thể escape bằng dấu gạch chéo.
func isPunctuation(c byte) bool {
	return strings.ContainsRune(`!"#$%&'()*+,-./:;<=>?@[\]^_{|}~`, rune(c))
}

// parseCodeSpan đọc code span dạng `...`.
func parseCodeSpan(text string, start int) (string, int, bool) {
	run := 1
	for start+run < len(text) && text[start+run] == '`' {
		run++
	}
	delimiter := strings.Repeat("`", run)
	rest := text[start+run:]
	idx := strings.Index(rest, delimiter)
	if idx < 0 {
		return "", start, false
	}
	content := rest[:idx]
	if len(content) >= 2 && strings.HasPrefix(content, " ") && strings.HasSuffix(content, " ") && strings.TrimSpace(content) != "" {
		content = content[1 : len(content)-1]
	}
	return "<code>" + html.EscapeString(content) + "</code>", start + run + idx + run, true
}

// parseLink đọc liên kết dạng [chữ](url "tiêu đề").
func (r *renderer) parseLink(text string, start int) (string, int, bool) {
	label, afterLabel, ok := readBracket(text, start)
	if !ok {
		return "", start, false
	}
	dest, title, next, ok := readDestination(text, afterLabel)
	if !ok {
		return "", start, false
	}
	if r.opts.Link != nil {
		if mapped := r.opts.Link(dest); mapped != "" {
			dest = mapped
		}
	}
	safe, ok := safeURL(dest)
	if !ok {
		return "", start, false
	}
	var out strings.Builder
	out.WriteString(`<a href="`)
	out.WriteString(safe)
	out.WriteString(`"`)
	if title != "" {
		out.WriteString(` title="`)
		out.WriteString(html.EscapeString(title))
		out.WriteString(`"`)
	}
	out.WriteString(">")
	out.WriteString(r.inline(label))
	out.WriteString("</a>")
	return out.String(), next, true
}

// parseImage đọc ảnh dạng ![mô tả](url).
func (r *renderer) parseImage(text string, start int) (string, int, bool) {
	label, afterLabel, ok := readBracket(text, start+1)
	if !ok {
		return "", start, false
	}
	dest, title, next, ok := readDestination(text, afterLabel)
	if !ok {
		return "", start, false
	}
	if r.opts.Image != nil {
		if mapped := r.opts.Image(dest); mapped != "" {
			dest = mapped
		}
	}
	safe, ok := safeURL(dest)
	if !ok {
		return "", start, false
	}
	var out strings.Builder
	out.WriteString(`<img src="`)
	out.WriteString(safe)
	out.WriteString(`" alt="`)
	out.WriteString(html.EscapeString(stripInline(label)))
	out.WriteString(`"`)
	if title != "" {
		out.WriteString(` title="`)
		out.WriteString(html.EscapeString(title))
		out.WriteString(`"`)
	}
	out.WriteString(">")
	return out.String(), next, true
}

// readBracket đọc nội dung trong cặp ngoặc vuông và trả vị trí sau dấu đóng.
func readBracket(text string, start int) (string, int, bool) {
	if start >= len(text) || text[start] != '[' {
		return "", start, false
	}
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '\\':
			i++
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return text[start+1 : i], i + 1, true
			}
		}
	}
	return "", start, false
}

// readDestination đọc phần (url "title") sau nhãn liên kết.
func readDestination(text string, start int) (string, string, int, bool) {
	if start >= len(text) || text[start] != '(' {
		return "", "", start, false
	}
	i := start + 1
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	var dest string
	if i < len(text) && text[i] == '<' {
		end := strings.IndexByte(text[i+1:], '>')
		if end < 0 {
			return "", "", start, false
		}
		dest = text[i+1 : i+1+end]
		i += end + 2
	} else {
		depth := 0
		begin := i
		for ; i < len(text); i++ {
			c := text[i]
			if c == '\\' {
				i++
				continue
			}
			if c == '(' {
				depth++
			}
			if c == ')' {
				if depth == 0 {
					break
				}
				depth--
			}
			if (c == ' ' || c == '\t') && depth == 0 {
				break
			}
		}
		dest = text[begin:i]
	}
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	title := ""
	if i < len(text) && (text[i] == '"' || text[i] == '\'' || text[i] == '(') {
		closer := text[i]
		if closer == '(' {
			closer = ')'
		}
		end := strings.IndexByte(text[i+1:], closer)
		if end < 0 {
			return "", "", start, false
		}
		title = text[i+1 : i+1+end]
		i += end + 2
		for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
			i++
		}
	}
	if i >= len(text) || text[i] != ')' {
		return "", "", start, false
	}
	return dest, title, i + 1, true
}

// parseAutolink đọc liên kết tự động dạng <https://...>.
func parseAutolink(text string, start int) (string, int, bool) {
	end := strings.IndexByte(text[start+1:], '>')
	if end < 0 {
		return "", start, false
	}
	target := text[start+1 : start+1+end]
	trimmed := strings.TrimSpace(target)
	if strings.HasPrefix(strings.ToLower(trimmed), "http://") || strings.HasPrefix(strings.ToLower(trimmed), "https://") {
		safe, ok := safeURL(trimmed)
		if !ok {
			return "", start, false
		}
		return `<a href="` + safe + `">` + html.EscapeString(trimmed) + `</a>`, start + end + 2, true
	}
	if strings.Contains(trimmed, "@") && !strings.ContainsAny(trimmed, " \t") {
		safe, ok := safeURL("mailto:" + trimmed)
		if !ok {
			return "", start, false
		}
		return `<a href="` + safe + `">` + html.EscapeString(trimmed) + `</a>`, start + end + 2, true
	}
	return "", start, false
}

// parseEmphasis đọc đậm, nghiêng và gạch ngang.
func (r *renderer) parseEmphasis(text string, start int) (string, int, bool) {
	marker := text[start]
	run := 1
	for start+run < len(text) && text[start+run] == marker {
		run++
	}
	if marker == '~' && run < 2 {
		return "", start, false
	}
	if marker == '_' && start > 0 && isWordByte(text[start-1]) {
		return "", start, false
	}
	use := 1
	tag := "em"
	switch {
	case run >= 2 && marker == '~':
		use = 2
		tag = "del"
	case run >= 2:
		use = 2
		tag = "strong"
	}
	delimiter := strings.Repeat(string(marker), use)
	searchFrom := start + use
	for i := searchFrom; i+use <= len(text); i++ {
		if text[i] == '\\' {
			i++
			continue
		}
		if !strings.HasPrefix(text[i:], delimiter) {
			continue
		}
		if marker == '_' && i+use < len(text) && isWordByte(text[i+use]) {
			continue
		}
		content := text[searchFrom:i]
		if strings.TrimSpace(content) == "" || strings.HasPrefix(content, " ") || strings.HasSuffix(content, " ") {
			continue
		}
		rendered := r.inline(content)
		return "<" + tag + ">" + rendered + "</" + tag + ">", i + use, true
	}
	return "", start, false
}

// isWordByte cho biết byte thuộc một từ để áp quy tắc nhấn mạnh.
func isWordByte(c byte) bool {
	return c == '_' || c == '-' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// safeURL kiểm tra và escape URL trước khi đưa vào thuộc tính.
func safeURL(raw string) (string, bool) {
	cleaned := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
	if cleaned == "" {
		return "", false
	}
	lower := strings.ToLower(cleaned)
	if strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "vbscript:") {
		return "", false
	}
	parsed, err := url.Parse(cleaned)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "", "http", "https", "mailto":
		return html.EscapeString(cleaned), true
	default:
		return "", false
	}
}

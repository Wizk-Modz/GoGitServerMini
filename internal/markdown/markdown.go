// Package markdown chuyển Markdown sang HTML an toàn mà không cần dependency ngoài.
package markdown

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// headingPattern nhận diện tiêu đề ATX.
var headingPattern = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)

// listPattern nhận diện mục danh sách có thụt lề.
var listPattern = regexp.MustCompile(`^(\s*)([-*+]|\d{1,9}[.)])\s+(.*)$`)

// taskPattern nhận diện checkbox đầu mục danh sách.
var taskPattern = regexp.MustCompile(`^\[([ xX])\]\s+(.*)$`)

// tableDividerPattern nhận diện dòng phân cách của bảng.
var tableDividerPattern = regexp.MustCompile(`^\s*\|?\s*:?-{1,}:?\s*(\|\s*:?-{1,}:?\s*)*\|?\s*$`)

// Options cho phép tầng gọi điều chỉnh URL tương đối trong nội dung.
type Options struct {
	// Link nhận URL gốc của liên kết và trả URL thay thế, rỗng để giữ nguyên.
	Link func(dest string) string
	// Image nhận URL gốc của ảnh và trả URL thay thế, rỗng để giữ nguyên.
	Image func(dest string) string
}

// renderer giữ tuỳ chọn khi duyệt Markdown.
type renderer struct {
	opts Options
}

// Render chuyển Markdown trong src thành HTML đã escape.
func Render(src []byte) string {
	return RenderWith(src, Options{})
}

// RenderWith chuyển Markdown trong src thành HTML với tuỳ chọn URL.
func RenderWith(src []byte, opts Options) string {
	text := strings.ReplaceAll(string(src), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var out strings.Builder
	r := &renderer{opts: opts}
	r.renderBlocks(&out, strings.Split(text, "\n"), false)
	return out.String()
}

// renderBlocks duyệt các dòng và sinh HTML theo từng khối.
func (r *renderer) renderBlocks(out *strings.Builder, lines []string, tight bool) {
	for i := 0; i < len(lines); {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}

		if fence, lang, ok := parseFence(line); ok {
			i = r.renderCodeFence(out, lines, i, fence, lang)
			continue
		}
		if level, content, ok := parseHeading(line); ok {
			r.renderHeading(out, level, content)
			i++
			continue
		}
		if isHorizontalRule(line) {
			out.WriteString("<hr>\n")
			i++
			continue
		}
		if strings.HasPrefix(strings.TrimLeft(line, " "), ">") {
			i = r.renderBlockquote(out, lines, i)
			continue
		}
		if indent, ordered, _, _, ok := parseListItem(line); ok {
			i = r.renderList(out, lines, i, indent, ordered)
			continue
		}
		if i+1 < len(lines) && isTableHeader(line, lines[i+1]) {
			i = r.renderTable(out, lines, i)
			continue
		}
		if level, ok := setextLevel(line, lines, i); ok {
			r.renderHeading(out, level, strings.TrimSpace(line))
			i += 2
			continue
		}
		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			i = r.renderIndentedCode(out, lines, i)
			continue
		}
		i = r.renderParagraph(out, lines, i, tight)
	}
}

// parseFence nhận diện dòng mở khối code và trả ký tự cùng ngôn ngữ.
func parseFence(line string) (string, string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return "", "", false
	}
	for _, char := range []byte{'`', '~'} {
		run := 0
		for run < len(trimmed) && trimmed[run] == char {
			run++
		}
		if run < 3 {
			continue
		}
		rest := strings.TrimSpace(trimmed[run:])
		lang := ""
		if rest != "" {
			lang = strings.Fields(rest)[0]
		}
		return trimmed[:run], lang, true
	}
	return "", "", false
}

// renderCodeFence gom nội dung tới dòng đóng và trả vị trí kế tiếp.
func (r *renderer) renderCodeFence(out *strings.Builder, lines []string, start int, fence, lang string) int {
	body := make([]string, 0, 8)
	i := start + 1
	for ; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, string(fence[0])) == "" {
			i++
			break
		}
		body = append(body, lines[i])
	}
	out.WriteString("<pre><code")
	if class := languageClass(lang); class != "" {
		out.WriteString(` class="language-`)
		out.WriteString(class)
		out.WriteString(`"`)
	}
	out.WriteString(">")
	out.WriteString(html.EscapeString(strings.Join(body, "\n")))
	out.WriteString("</code></pre>\n")
	return i
}

// languageClass lọc tên ngôn ngữ để dùng an toàn trong thuộc tính class.
func languageClass(lang string) string {
	var b strings.Builder
	for _, r := range lang {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '+' || r == '#' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// parseHeading tách cấp và nội dung tiêu đề ATX.
func parseHeading(line string) (int, string, bool) {
	match := headingPattern.FindStringSubmatch(line)
	if match == nil {
		return 0, "", false
	}
	return len(match[1]), match[2], true
}

// renderHeading ghi tiêu đề kèm id neo ổn định.
func (r *renderer) renderHeading(out *strings.Builder, level int, content string) {
	slug := Slug(content)
	out.WriteString("<h")
	out.WriteString(strconv.Itoa(level))
	if slug != "" {
		out.WriteString(` id="`)
		out.WriteString(html.EscapeString(slug))
		out.WriteString(`"`)
	}
	out.WriteString(">")
	out.WriteString(r.inline(content))
	out.WriteString("</h")
	out.WriteString(strconv.Itoa(level))
	out.WriteString(">\n")
}

// Slug tạo id neo từ nội dung tiêu đề.
func Slug(text string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(stripInline(text)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// stripInline bỏ các ký hiệu định dạng để lấy chữ thuần.
func stripInline(text string) string {
	replacer := strings.NewReplacer("*", "", "`", "", "~", "", "[", "", "]", "", "(", "", ")", "")
	return replacer.Replace(text)
}

// isHorizontalRule nhận diện đường kẻ ngang.
func isHorizontalRule(line string) bool {
	trimmed := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
	if len(trimmed) < 3 {
		return false
	}
	first := trimmed[0]
	if first != '-' && first != '*' && first != '_' {
		return false
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] != first {
			return false
		}
	}
	return true
}

// setextLevel nhận diện tiêu đề gạch chân bằng dấu = hoặc -.
func setextLevel(line string, lines []string, i int) (int, bool) {
	if i+1 >= len(lines) || strings.TrimSpace(line) == "" {
		return 0, false
	}
	next := strings.TrimSpace(lines[i+1])
	if next == "" || strings.Contains(next, " ") {
		return 0, false
	}
	if strings.Trim(next, "=") == "" && len(next) >= 1 {
		return 1, true
	}
	if strings.Trim(next, "-") == "" && len(next) >= 2 {
		return 2, true
	}
	return 0, false
}

// renderBlockquote gom các dòng trích dẫn rồi render đệ quy.
func (r *renderer) renderBlockquote(out *strings.Builder, lines []string, start int) int {
	body := make([]string, 0, 4)
	i := start
	for ; i < len(lines); i++ {
		trimmed := strings.TrimLeft(lines[i], " ")
		if !strings.HasPrefix(trimmed, ">") {
			if strings.TrimSpace(lines[i]) == "" && i+1 < len(lines) && strings.HasPrefix(strings.TrimLeft(lines[i+1], " "), ">") {
				body = append(body, "")
				continue
			}
			break
		}
		content := strings.TrimPrefix(trimmed, ">")
		content = strings.TrimPrefix(content, " ")
		body = append(body, content)
	}
	out.WriteString("<blockquote>\n")
	r.renderBlocks(out, body, false)
	out.WriteString("</blockquote>\n")
	return i
}

// parseListItem tách thụt lề, kiểu danh sách và nội dung mục.
func parseListItem(line string) (indent int, ordered bool, start int, content string, ok bool) {
	match := listPattern.FindStringSubmatch(line)
	if match == nil {
		return 0, false, 0, "", false
	}
	indent = len(strings.ReplaceAll(match[1], "\t", "    "))
	marker := match[2]
	if marker[0] >= '0' && marker[0] <= '9' {
		ordered = true
		if value, err := strconv.Atoi(strings.TrimRight(marker, ".)")); err == nil {
			start = value
		}
	}
	return indent, ordered, start, match[3], true
}

// renderList dựng danh sách lồng nhau theo mức thụt lề.
func (r *renderer) renderList(out *strings.Builder, lines []string, start, baseIndent int, ordered bool) int {
	tag := "ul"
	if ordered {
		tag = "ol"
	}
	out.WriteString("<" + tag)
	if ordered {
		if _, _, first, _, ok := parseListItem(lines[start]); ok && first != 1 {
			out.WriteString(` start="` + strconv.Itoa(first) + `"`)
		}
	}
	out.WriteString(">\n")

	i := start
	for i < len(lines) {
		indent, itemOrdered, _, content, ok := parseListItem(lines[i])
		if !ok || indent != baseIndent || itemOrdered != ordered {
			break
		}
		itemLines := []string{content}
		i++
		// Gom các dòng tiếp theo thuộc cùng mục, gồm cả danh sách con.
		for i < len(lines) {
			if strings.TrimSpace(lines[i]) == "" {
				if i+1 < len(lines) {
					nextIndent, _, _, _, nextOK := parseListItem(lines[i+1])
					if nextOK && nextIndent > baseIndent {
						itemLines = append(itemLines, "")
						i++
						continue
					}
				}
				break
			}
			nextIndent, _, _, _, nextOK := parseListItem(lines[i])
			if nextOK && nextIndent <= baseIndent {
				break
			}
			leading := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
			if leading <= baseIndent && !strings.HasPrefix(lines[i], "  ") {
				break
			}
			itemLines = append(itemLines, trimIndent(lines[i], baseIndent+2))
			i++
		}
		out.WriteString("<li>")
		if checkbox, label, ok := parseTaskItem(content); ok {
			out.WriteString(`<input type="checkbox" disabled`)
			if checkbox {
				out.WriteString(" checked")
			}
			out.WriteString(`> `)
			itemLines[0] = label
		}
		var inner strings.Builder
		r.renderBlocks(&inner, itemLines, true)
		out.WriteString(stripParagraph(inner.String()))
		out.WriteString("</li>\n")
	}
	out.WriteString("</" + tag + ">\n")
	return i
}

// parseTaskItem nhận diện checkbox đầu mục danh sách.
func parseTaskItem(content string) (bool, string, bool) {
	match := taskPattern.FindStringSubmatch(content)
	if match == nil {
		return false, "", false
	}
	return strings.EqualFold(match[1], "x"), match[2], true
}

// trimIndent bỏ tối đa n ký tự thụt lề ở đầu dòng.
func trimIndent(line string, n int) string {
	removed := 0
	for removed < n && removed < len(line) && line[removed] == ' ' {
		removed++
	}
	return line[removed:]
}

// stripParagraph bỏ cặp thẻ p duy nhất để danh sách gọn như GitHub.
func stripParagraph(rendered string) string {
	trimmed := strings.TrimSpace(rendered)
	if strings.HasPrefix(trimmed, "<p>") && strings.HasSuffix(trimmed, "</p>") &&
		strings.Count(trimmed, "<p>") == 1 {
		return strings.TrimSuffix(strings.TrimPrefix(trimmed, "<p>"), "</p>")
	}
	return trimmed
}

// isTableHeader kiểm tra cặp dòng đầu bảng và dòng phân cách.
func isTableHeader(line, next string) bool {
	if !strings.Contains(line, "|") {
		return false
	}
	if !tableDividerPattern.MatchString(next) || !strings.Contains(next, "-") {
		return false
	}
	return len(splitRow(line)) == len(splitRow(next))
}

// splitRow tách ô của một dòng bảng.
func splitRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	parts := strings.Split(trimmed, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// renderTable dựng bảng có header và căn lề theo dòng phân cách.
func (r *renderer) renderTable(out *strings.Builder, lines []string, start int) int {
	header := splitRow(lines[start])
	divider := splitRow(lines[start+1])
	aligns := make([]string, len(divider))
	for i, cell := range divider {
		left := strings.HasPrefix(cell, ":")
		right := strings.HasSuffix(cell, ":")
		switch {
		case left && right:
			aligns[i] = "center"
		case right:
			aligns[i] = "right"
		case left:
			aligns[i] = "left"
		}
	}
	cellTag := func(tag string, i int, content string) {
		out.WriteString("<" + tag)
		if i < len(aligns) && aligns[i] != "" {
			out.WriteString(` style="text-align:` + aligns[i] + `"`)
		}
		out.WriteString(">")
		out.WriteString(r.inline(content))
		out.WriteString("</" + tag + ">")
	}

	out.WriteString("<table>\n<thead>\n<tr>")
	for i, cell := range header {
		cellTag("th", i, cell)
	}
	out.WriteString("</tr>\n</thead>\n<tbody>\n")
	i := start + 2
	for ; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" || !strings.Contains(lines[i], "|") {
			break
		}
		out.WriteString("<tr>")
		for idx, cell := range splitRow(lines[i]) {
			cellTag("td", idx, cell)
		}
		out.WriteString("</tr>\n")
	}
	out.WriteString("</tbody>\n</table>\n")
	return i
}

// renderIndentedCode gom khối code thụt lề 4 dấu cách.
func (r *renderer) renderIndentedCode(out *strings.Builder, lines []string, start int) int {
	body := make([]string, 0, 4)
	i := start
	for ; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			body = append(body, "")
			continue
		}
		if !strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "\t") {
			break
		}
		body = append(body, strings.TrimPrefix(strings.TrimPrefix(line, "\t"), "    "))
	}
	out.WriteString("<pre><code>")
	out.WriteString(html.EscapeString(strings.TrimRight(strings.Join(body, "\n"), "\n")))
	out.WriteString("</code></pre>\n")
	return i
}

// renderParagraph gom dòng liên tiếp thành một đoạn văn.
func (r *renderer) renderParagraph(out *strings.Builder, lines []string, start int, tight bool) int {
	body := make([]string, 0, 4)
	i := start
	for ; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			break
		}
		if i > start && startsBlock(lines, i) {
			break
		}
		body = append(body, strings.TrimSpace(line))
	}
	rendered := r.inline(strings.Join(body, "\n"))
	if tight && len(body) == 1 && !strings.Contains(rendered, "<br>") {
		out.WriteString(rendered)
		out.WriteString("\n")
		return i
	}
	out.WriteString("<p>")
	out.WriteString(rendered)
	out.WriteString("</p>\n")
	return i
}

// startsBlock cho biết dòng bắt đầu một khối mới.
func startsBlock(lines []string, i int) bool {
	line := lines[i]
	if _, _, ok := parseFence(line); ok {
		return true
	}
	if _, _, ok := parseHeading(line); ok {
		return true
	}
	if isHorizontalRule(line) {
		return true
	}
	if strings.HasPrefix(strings.TrimLeft(line, " "), ">") {
		return true
	}
	if _, _, _, _, ok := parseListItem(line); ok {
		return true
	}
	if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
		return true
	}
	return i+1 < len(lines) && isTableHeader(line, lines[i+1])
}

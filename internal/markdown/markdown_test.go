package markdown

import (
	"strings"
	"testing"
)

func TestHeading(t *testing.T) {
	out := Render([]byte("# Tiêu đề\n\n## Cấp hai\n"))
	for _, want := range []string{`<h1 id="tiêu-đề">Tiêu đề</h1>`, `<h2 id="cấp-hai">Cấp hai</h2>`} {
		if !strings.Contains(out, want) {
			t.Errorf("thiếu %q trong:\n%s", want, out)
		}
	}
}

func TestParagraphAndInline(t *testing.T) {
	out := Render([]byte("Đoạn **đậm**, *nghiêng*, ~~xoá~~ và `code`.\n"))
	for _, want := range []string{
		"<p>Đoạn <strong>đậm</strong>, <em>nghiêng</em>, <del>xoá</del> và <code>code</code>.</p>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("thiếu %q trong:\n%s", want, out)
		}
	}
}

func TestLinksAndImages(t *testing.T) {
	out := Render([]byte("[link](https://example.com \"tiêu đề\") và ![ảnh](https://example.com/a.png)\n"))
	for _, want := range []string{
		`<a href="https://example.com" title="tiêu đề">link</a>`,
		`<img src="https://example.com/a.png" alt="ảnh">`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("thiếu %q trong:\n%s", want, out)
		}
	}
}

func TestUnsafeURLRejected(t *testing.T) {
	cases := []string{
		"[x](javascript:alert(1))",
		"[x](JavaScript:alert(1))",
		"[x](data:text/html;base64,PHNjcmlwdD4=)",
		"[x](vbscript:msgbox(1))",
		"![x](javascript:alert(1))",
	}
	for _, input := range cases {
		out := Render([]byte(input + "\n"))
		if strings.Contains(out, "<a ") || strings.Contains(out, "<img ") {
			t.Errorf("%q tạo thẻ nguy hiểm:\n%s", input, out)
		}
	}
}

func TestRawHTMLIsEscaped(t *testing.T) {
	out := Render([]byte("<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>\n"))
	if strings.Contains(out, "<script") || strings.Contains(out, "<img") {
		t.Errorf("HTML thô chưa escape:\n%s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Errorf("thiếu escape script:\n%s", out)
	}
}

func TestCodeFence(t *testing.T) {
	out := Render([]byte("```go\nfmt.Println(\"<x>\")\n```\n"))
	if !strings.Contains(out, `<pre><code class="language-go">`) {
		t.Errorf("thiếu class ngôn ngữ:\n%s", out)
	}
	if strings.Contains(out, "<x>") {
		t.Errorf("code chưa escape:\n%s", out)
	}
	if !strings.Contains(out, "fmt.Println(&#34;&lt;x&gt;&#34;)") {
		t.Errorf("nội dung code sai:\n%s", out)
	}
}

func TestCodeFenceLongerDelimiter(t *testing.T) {
	out := Render([]byte("````\ncode with ``` inside\n````\n"))
	if !strings.Contains(out, "code with ``` inside") {
		t.Errorf("fence dài không giữ nội dung:\n%s", out)
	}
}

func TestUnclosedFence(t *testing.T) {
	out := Render([]byte("```\nkhông đóng\n"))
	if !strings.Contains(out, "<pre><code>") || !strings.Contains(out, "không đóng") {
		t.Errorf("fence chưa đóng xử lý sai:\n%s", out)
	}
}

func TestLists(t *testing.T) {
	out := Render([]byte("- một\n- hai\n  - con\n"))
	for _, want := range []string{"<ul>", "<li>một</li>", "<li>hai", "<li>con</li>"} {
		if !strings.Contains(out, want) {
			t.Errorf("thiếu %q trong:\n%s", want, out)
		}
	}

	ordered := Render([]byte("3. ba\n4. bốn\n"))
	if !strings.Contains(ordered, `<ol start="3">`) {
		t.Errorf("thiếu start=3:\n%s", ordered)
	}
}

func TestTaskList(t *testing.T) {
	out := Render([]byte("- [x] xong\n- [ ] chưa\n"))
	if !strings.Contains(out, `type="checkbox" disabled checked`) {
		t.Errorf("thiếu checkbox đã chọn:\n%s", out)
	}
	if !strings.Contains(out, `type="checkbox" disabled>`) {
		t.Errorf("thiếu checkbox chưa chọn:\n%s", out)
	}
}

func TestTable(t *testing.T) {
	out := Render([]byte("| A | B |\n|---|---:|\n| 1 | 2 |\n"))
	for _, want := range []string{"<table>", "<th>A</th>", `<th style="text-align:right">B</th>`, "<td>1</td>"} {
		if !strings.Contains(out, want) {
			t.Errorf("thiếu %q trong:\n%s", want, out)
		}
	}
}

func TestBlockquote(t *testing.T) {
	out := Render([]byte("> trích dẫn\n> dòng hai\n"))
	if !strings.Contains(out, "<blockquote>") || !strings.Contains(out, "trích dẫn") {
		t.Errorf("blockquote sai:\n%s", out)
	}
}

func TestHorizontalRule(t *testing.T) {
	out := Render([]byte("trước\n\n---\n\nsau\n"))
	if !strings.Contains(out, "<hr>") {
		t.Errorf("thiếu hr:\n%s", out)
	}
}

func TestAmpersandAndSpecialChars(t *testing.T) {
	out := Render([]byte("a & b < c > d\n"))
	if !strings.Contains(out, "a &amp; b &lt; c &gt; d") {
		t.Errorf("escape ký tự đặc biệt sai:\n%s", out)
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Xin chào":         "xin-chào",
		"Tiêu đề  có  dấu": "tiêu-đề-có-dấu",
		"`code` **đậm**":   "code-đậm",
		"!!!":              "",
	}
	for input, want := range cases {
		if got := Slug(input); got != want {
			t.Errorf("Slug(%q) = %q, muốn %q", input, got, want)
		}
	}
}

func TestEmptyInput(t *testing.T) {
	if out := Render(nil); out != "" {
		t.Errorf("Render(nil) = %q, muốn rỗng", out)
	}
}

func TestCRLF(t *testing.T) {
	out := Render([]byte("# Tiêu đề\r\n\r\nĐoạn văn\r\n"))
	if !strings.Contains(out, "<h1") || !strings.Contains(out, "<p>Đoạn văn</p>") {
		t.Errorf("CRLF xử lý sai:\n%s", out)
	}
}

func TestRenderWithLinkImageHooks(t *testing.T) {
	opts := Options{
		Link: func(dest string) string {
			if strings.HasPrefix(dest, "#") || strings.Contains(dest, "://") {
				return ""
			}
			return "/repo/demo/blob/" + strings.TrimPrefix(dest, "./")
		},
		Image: func(dest string) string {
			if strings.Contains(dest, "://") {
				return ""
			}
			return "/repo/demo/raw/" + strings.TrimPrefix(dest, "./")
		},
	}
	out := RenderWith([]byte("[tài liệu](docs/a.md) [ngoài](https://example.com) ![ảnh](img/a.png)\n"), opts)
	for _, want := range []string{
		`<a href="/repo/demo/blob/docs/a.md">tài liệu</a>`,
		`<a href="https://example.com">ngoài</a>`,
		`<img src="/repo/demo/raw/img/a.png" alt="ảnh">`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("thiếu %q trong:\n%s", want, out)
		}
	}
}

func TestRenderWithHookCannotInjectUnsafeURL(t *testing.T) {
	opts := Options{Link: func(string) string { return "javascript:alert(1)" }}
	out := RenderWith([]byte("[x](a.md)\n"), opts)
	if strings.Contains(out, "javascript:") || strings.Contains(out, "<a ") {
		t.Errorf("hook trả URL nguy hiểm nhưng vẫn render:\n%s", out)
	}
}

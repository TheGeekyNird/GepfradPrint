package render

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Document struct {
	Data []byte
	Name string
	MIME string
}

func Load(path string) (Document, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return Document{}, e
	}
	ext := strings.ToLower(filepath.Ext(path))
	mime := "application/octet-stream"
	switch ext {
	case ".txt":
		mime = "text/plain"
	case ".pdf":
		mime = "application/pdf"
	case ".html":
		mime = "text/html"
	}
	return Document{b, filepath.Base(path), mime}, nil
}
func Prepare(d Document) ([]byte, string, error) {
	if d.MIME == "application/pdf" || bytes.HasPrefix(d.Data, []byte("%PDF-")) {
		return d.Data, "application/pdf", nil
	}
	if d.MIME == "text/plain" || d.MIME == "text/html" {
		text := string(d.Data)
		if d.MIME == "text/html" {
			text = stripHTML(text)
		}
		return textPDF(text), "application/pdf", nil
	}
	return nil, "", fmt.Errorf("no renderer for %s", d.MIME)
}
func stripHTML(s string) string {
	var out strings.Builder
	in := false
	for _, r := range s {
		if r == '<' {
			in = true
			continue
		}
		if r == '>' {
			in = false
			out.WriteByte(' ')
			continue
		}
		if !in {
			out.WriteRune(r)
		}
	}
	return strings.TrimSpace(out.String())
}
func esc(s string) string {
	return strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(s)
}
func textPDF(text string) []byte {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var content strings.Builder
	content.WriteString("BT /F1 10 Tf 54 750 Td ")
	for i, line := range lines {
		if i > 0 {
			content.WriteString("0 -14 Td ")
		}
		content.WriteString("(" + esc(line) + ") Tj ")
		if (i+1)%50 == 0 {
			content.WriteString("ET BT /F1 10 Tf 54 750 Td ")
		}
	}
	content.WriteString("ET")
	stream := content.String()
	objs := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>", fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream)}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	offs := []int{0}
	for i, o := range objs {
		offs = append(offs, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offs[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

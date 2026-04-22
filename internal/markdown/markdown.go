package markdown

import (
	"bytes"
	"html"

	"github.com/yuin/goldmark"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

var md = goldmark.New(
	goldmark.WithRendererOptions(
		goldmarkhtml.WithHardWraps(),
	),
)

func ToHTML(src string) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return html.EscapeString(src)
	}
	return buf.String()
}

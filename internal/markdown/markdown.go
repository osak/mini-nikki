package markdown

import (
	"bytes"
	"html"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

var md = goldmark.New(
	goldmark.WithExtensions(
		// 本文中の bare URL をリンクにする。
		extension.NewLinkify(linkifyOptions...),
		cjkLinkify{},
	),
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

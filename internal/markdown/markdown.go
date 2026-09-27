package markdown

import (
	"bytes"
	"html"

	"github.com/osak/mini-nikki/internal/linkpreview"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

var md = goldmark.New(
	goldmark.WithExtensions(
		// 本文中の bare URL をリンクにする。
		extension.NewLinkify(linkifyOptions...),
		cjkLinkify{},
		// @[card](url) / @[embed](url)
		linkEmbedExtension{},
	),
	goldmark.WithRendererOptions(
		goldmarkhtml.WithHardWraps(),
	),
)

// Options は ToHTML の描画オプション。
type Options struct {
	// Previews はリンクカードに使う取得済みの OGP。キーは URL。
	// 含まれない URL のカードはフォールバック表示になる。
	Previews map[string]linkpreview.Preview
	// Plain はフィード向けの描画。カードや埋め込みをただのリンクにする。
	Plain bool
}

func ToHTML(src string, opts Options) string {
	source := []byte(src)
	doc := md.Parser().Parse(text.NewReader(source))

	// goldmark のレンダラは呼び出しごとの情報を受け取れないので、
	// 描画前にノードへ直接書き込む。
	walkLinkEmbeds(doc, func(n *LinkEmbed) {
		n.plain = opts.Plain
		if p, ok := opts.Previews[n.URL]; ok {
			n.preview = &p
		}
	})

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, source, doc); err != nil {
		return html.EscapeString(src)
	}
	return buf.String()
}

// ExtractEmbedURLs は本文中の @[card] / @[embed] の URL を出現順に重複なく返す。
func ExtractEmbedURLs(src string) []string {
	doc := md.Parser().Parse(text.NewReader([]byte(src)))
	var urls []string
	seen := map[string]bool{}
	walkLinkEmbeds(doc, func(n *LinkEmbed) {
		if !seen[n.URL] {
			seen[n.URL] = true
			urls = append(urls, n.URL)
		}
	})
	return urls
}

// walkLinkEmbeds は n の子孫の LinkEmbed ノードを出現順に fn に渡す。
func walkLinkEmbeds(n ast.Node, fn func(*LinkEmbed)) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if e, ok := c.(*LinkEmbed); ok {
			fn(e)
			continue
		}
		walkLinkEmbeds(c, fn)
	}
}

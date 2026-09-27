package markdown

import (
	"context"
	"net/url"
	"regexp"

	"github.com/a-h/templ"
	"github.com/osak/mini-nikki/internal/linkpreview"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// EmbedKind は埋め込み記法の種類。
type EmbedKind string

const (
	// KindCard は @[card](url)。OGP のリンクカードとして表示する。
	KindCard EmbedKind = "card"
	// KindEmbed は @[embed](url)。対応サイトなら埋め込みプレーヤーとして、
	// それ以外はリンクカードとして表示する。
	KindEmbed EmbedKind = "embed"
)

// KindLinkEmbed は LinkEmbed ノードの種類。
var KindLinkEmbed = ast.NewNodeKind("LinkEmbed")

// LinkEmbed は行単独で書かれた @[card](url) / @[embed](url) を表すブロックノード。
type LinkEmbed struct {
	ast.BaseBlock
	Mode EmbedKind
	URL  string

	// 以下は描画直前に ToHTML が設定する。
	preview *linkpreview.Preview
	plain   bool
}

func (n *LinkEmbed) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Mode": string(n.Mode), "URL": n.URL}, nil)
}

func (n *LinkEmbed) Kind() ast.NodeKind { return KindLinkEmbed }

// linkEmbedRegexp は埋め込み記法の 1 行にマッチする。前後の空白は許す。
var linkEmbedRegexp = regexp.MustCompile(`^[ \t]{0,3}@\[(card|embed)\]\((https?://[^\s()]+)\)[ \t]*\r?\n?$`)

// parseLinkEmbedLine は line が埋め込み記法ならその種類と URL を返す。
func parseLinkEmbedLine(line []byte) (EmbedKind, string, bool) {
	m := linkEmbedRegexp.FindSubmatch(line)
	if m == nil {
		return "", "", false
	}
	u, err := url.Parse(string(m[2]))
	if err != nil || u.Host == "" {
		return "", "", false
	}
	return EmbedKind(m[1]), string(m[2]), true
}

type linkEmbedParser struct{}

func (linkEmbedParser) Trigger() []byte { return []byte{'@'} }

func (linkEmbedParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, seg := reader.PeekLine()
	kind, u, ok := parseLinkEmbedLine(line)
	if !ok {
		return nil, parser.NoChildren
	}
	// 改行の手前まで進める（改行ごと読むと、次の行が AdvanceLine で読み飛ばされる）。
	n := seg.Len()
	if n > 0 && line[n-1] == '\n' {
		n--
	}
	reader.Advance(n)
	return &LinkEmbed{Mode: kind, URL: u}, parser.NoChildren
}

func (linkEmbedParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	// 1 行で完結する。
	return parser.Close
}

func (linkEmbedParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

// 段落の途中の行でも、行単独ならカードとして扱う（ハードラップ前提の日記では
// 空行を挟まずに書くことが多いため）。
func (linkEmbedParser) CanInterruptParagraph() bool { return true }

func (linkEmbedParser) CanAcceptIndentedLine() bool { return false }

type linkEmbedRenderer struct{}

func (linkEmbedRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindLinkEmbed, renderLinkEmbed)
}

func renderLinkEmbed(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	if err := linkEmbedView(node.(*LinkEmbed)).Render(context.Background(), w); err != nil {
		return ast.WalkStop, err
	}
	// ブロック要素として他の要素と同じく改行で終える。
	return ast.WalkSkipChildren, w.WriteByte('\n')
}

// linkEmbedView は n の描画に使うコンポーネントを選ぶ。マークアップは embed.templ にある。
func linkEmbedView(n *LinkEmbed) templ.Component {
	if n.plain {
		label := n.URL
		if n.preview != nil && n.preview.Title != "" {
			label = n.preview.Title
		}
		return feedLink(n.URL, label)
	}
	if n.Mode == KindEmbed {
		if c, ok := providerEmbed(n.URL); ok {
			return c
		}
		// 対応していないサイトはカードにフォールバックする。
	}
	return linkCard(newCard(n.URL, n.preview))
}

// card は linkCard に渡す表示内容。
type card struct {
	URL         string
	Title       string
	Description string
	SiteName    string
	ImageURL    string
	// Fallback は OGP が未取得・取得失敗のとき true。URL とドメインだけを出す。
	Fallback bool
}

// newCard は OGP の取得結果からカードの表示内容を作る。p が nil ならフォールバック表示にする。
func newCard(rawURL string, p *linkpreview.Preview) card {
	c := card{URL: rawURL, Title: rawURL, SiteName: rawURL, Fallback: p == nil}
	if u, err := url.Parse(rawURL); err == nil {
		c.SiteName = u.Host
	}
	if p == nil {
		return c
	}
	if p.Title != "" {
		c.Title = p.Title
	}
	if p.SiteName != "" {
		c.SiteName = p.SiteName
	}
	c.Description = p.Description
	c.ImageURL = p.ImageURL
	return c
}

// linkEmbedExtension は埋め込み記法を有効にする goldmark 拡張。
type linkEmbedExtension struct{}

func (linkEmbedExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithBlockParsers(
		// 段落（1000）より先に試す。
		util.Prioritized(linkEmbedParser{}, 750),
	))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(linkEmbedRenderer{}, 500),
	))
}

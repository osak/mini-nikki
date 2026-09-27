package markdown

import (
	"bytes"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// cjkLinkify は和文の直後に置かれた URL もリンクにする goldmark 拡張。
//
// goldmark の Linkify は URL の直前が空白か `*_~(` のときにしか発火しないため、
// 「（https://example.com）」や「見てhttps://example.com」がリンクにならない。
// goldmark はインラインパーサを行頭・空白・ASCII 記号の位置でしか呼ばないので、
// パース後の AST を走査して、非 ASCII 文字の直後にある URL を AutoLink に置き換える。
//
// URL の終端判定（末尾の句読点の除去など）は Linkify のパーサにそのまま委譲する。
// Linkify の正規表現は ASCII のみにマッチするので、全角の閉じ括弧や和文が続いても
// そこで URL は終わる。
type cjkLinkify struct{}

func (cjkLinkify) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			// Linkify 本体の処理が済んだ後に走ればよいので優先度は低めにする。
			util.Prioritized(&cjkLinkifyTransformer{linkify: extension.NewLinkifyParser()}, 999),
		),
	)
}

type cjkLinkifyTransformer struct {
	linkify parser.InlineParser
}

var linkPrefixes = [][]byte{[]byte("http://"), []byte("https://")}

func (t *cjkLinkifyTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()

	var texts []*ast.Text
	collectTexts(doc, &texts)

	for _, n := range texts {
		// mergeFollowingTexts で前のノードに併合されたものは親から外れている。
		if n.Parent() == nil || n.IsRaw() {
			continue
		}
		t.linkifyText(n, source, pc)
	}
}

// collectTexts は n の子孫のうち、リンク化の対象になるテキストノードを texts に集める。
// 変換中に AST を書き換えるので、先に対象を集めてから処理する。
func collectTexts(n ast.Node, texts *[]*ast.Text) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Link, *ast.AutoLink, *ast.Image, *ast.CodeSpan:
			// リンクの中にリンクは作らない。コード内もそのまま。
			continue
		case *ast.Text:
			*texts = append(*texts, c)
			continue
		}
		collectTexts(c, texts)
	}
}

// linkifyText は n の中にある「非 ASCII 文字の直後の URL」を AutoLink に分割する。
func (t *cjkLinkifyTransformer) linkifyText(n *ast.Text, source []byte, pc parser.Context) {
	parent := n.Parent()
	from := n.Segment.Start
	for {
		seg := n.Segment
		start := findCJKPrecededURL(source, text.NewSegment(from, seg.Stop))
		if start < 0 {
			return
		}
		mergeFollowingTexts(n)
		seg = n.Segment

		lines := text.NewSegments()
		lines.Append(text.NewSegment(start, seg.Stop))
		block := text.NewBlockReader(source, lines)
		link, ok := t.linkify.Parse(parent, block, pc).(*ast.AutoLink)
		if !ok {
			// 接頭辞はあったが URL として成立しなかった。その先から探し直す。
			from = start + 1
			continue
		}
		_, pos := block.Position()
		stop := pos.Start

		// n を [seg.Start, start) / link / [stop, seg.Stop) に分割する。
		// 行末の改行フラグは後ろ側のテキストに引き継ぐ。
		rest := ast.NewTextSegment(text.NewSegment(stop, seg.Stop))
		rest.SetSoftLineBreak(n.SoftLineBreak())
		rest.SetHardLineBreak(n.HardLineBreak())
		n.Segment = seg.WithStop(start)
		n.SetSoftLineBreak(false)
		n.SetHardLineBreak(false)
		parent.InsertAfter(parent, n, link)
		parent.InsertAfter(parent, link, rest)
		if start == seg.Start {
			parent.RemoveChild(parent, n)
		}
		n = rest
		from = stop
	}
}

// mergeFollowingTexts は n の直後に続く、ソース上で隙間なく連続したテキストノードを
// n に併合する。goldmark は `_` や `*` のような強調記号候補の位置でテキストノードを
// 分割するため、併合しないと URL の途中で打ち切られてしまう。
func mergeFollowingTexts(n *ast.Text) {
	for !n.SoftLineBreak() && !n.HardLineBreak() {
		next, ok := n.NextSibling().(*ast.Text)
		if !ok || next.IsRaw() || next.Segment.Start != n.Segment.Stop {
			return
		}
		n.Segment = n.Segment.WithStop(next.Segment.Stop)
		n.SetSoftLineBreak(next.SoftLineBreak())
		n.SetHardLineBreak(next.HardLineBreak())
		n.Parent().RemoveChild(n.Parent(), next)
	}
}

// findCJKPrecededURL は seg の範囲で、直前が非 ASCII 文字である URL の開始位置を返す。
// 見つからなければ -1。
func findCJKPrecededURL(source []byte, seg text.Segment) int {
	for i := seg.Start; i < seg.Stop; i++ {
		c := source[i]
		if c != 'h' && c != 'w' {
			continue
		}
		if !hasLinkPrefix(source[i:seg.Stop]) {
			continue
		}
		r, _ := utf8.DecodeLastRune(source[:i])
		if r != utf8.RuneError && r >= utf8.RuneSelf {
			return i
		}
	}
	return -1
}

func hasLinkPrefix(b []byte) bool {
	for _, prefix := range linkPrefixes {
		if bytes.HasPrefix(b, prefix) {
			return true
		}
	}
	return false
}

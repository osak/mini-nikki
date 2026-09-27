package markdown

import (
	"strings"
	"testing"
)

func TestToHTMLLinkify(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "空白で区切られた URL",
			src:  "見て https://example.com/a?b=1 です",
			want: `<p>見て <a href="https://example.com/a?b=1">https://example.com/a?b=1</a> です</p>`,
		},
		{
			name: "行頭の URL",
			src:  "https://example.com/",
			want: `<p><a href="https://example.com/">https://example.com/</a></p>`,
		},
		{
			name: "URL の直後に和文",
			src:  "https://example.com/あいうを見て",
			want: `<p><a href="https://example.com/">https://example.com/</a>あいうを見て</p>`,
		},
		{
			name: "末尾のピリオドは含めない",
			src:  "see https://example.com.",
			want: `<p>see <a href="https://example.com">https://example.com</a>.</p>`,
		},
		{
			name: "全角括弧で囲まれた URL",
			src:  "（https://example.com/foo）",
			want: `<p>（<a href="https://example.com/foo">https://example.com/foo</a>）</p>`,
		},
		{
			name: "かぎ括弧で囲まれた URL",
			src:  "「https://example.com/foo」",
			want: `<p>「<a href="https://example.com/foo">https://example.com/foo</a>」</p>`,
		},
		{
			name: "和文の直後に空白なしの URL",
			src:  "ここ：https://example.com/foo を参照",
			want: `<p>ここ：<a href="https://example.com/foo">https://example.com/foo</a> を参照</p>`,
		},
		{
			name: "和文の直後の URL で末尾の句読点を除く",
			src:  "見てhttps://example.com/foo.",
			want: `<p>見て<a href="https://example.com/foo">https://example.com/foo</a>.</p>`,
		},
		{
			name: "www. はリンクにしない",
			src:  "see www.example.com",
			want: `<p>see www.example.com</p>`,
		},
		{
			name: "行頭の www. もリンクにしない",
			src:  "www.example.com/foo",
			want: `<p>www.example.com/foo</p>`,
		},
		{
			name: "http:// 付きの www はリンクにする",
			src:  "see http://www.example.com",
			want: `<p>see <a href="http://www.example.com">http://www.example.com</a></p>`,
		},
		{
			name: "和文の直後の www. はリンクにしない",
			src:  "（www.example.com）",
			want: `<p>（www.example.com）</p>`,
		},
		{
			name: "和文の直後の http 以外の h は通常のテキスト",
			src:  "日本hello",
			want: `<p>日本hello</p>`,
		},
		{
			name: "和文の直後の URL が 1 行に複数",
			src:  "前https://a.example.com/中https://b.example.com/後",
			want: `<p>前<a href="https://a.example.com/">https://a.example.com/</a>中<a href="https://b.example.com/">https://b.example.com/</a>後</p>`,
		},
		{
			name: "和文の直後の URL にアンダースコアを含む",
			src:  "（https://example.com/a_b_c?x=1_2）",
			want: `<p>（<a href="https://example.com/a_b_c?x=1_2">https://example.com/a_b_c?x=1_2</a>）</p>`,
		},
		{
			name: "和文の直後の URL の後の改行を保つ",
			src:  "（https://example.com/）\n次の行",
			want: "<p>（<a href=\"https://example.com/\">https://example.com/</a>）<br>\n次の行</p>",
		},
		{
			name: "強調の中でも和文の直後の URL をリンクにする",
			src:  "**見てhttps://example.com/**",
			want: `<p><strong>見て<a href="https://example.com/">https://example.com/</a></strong></p>`,
		},
		{
			name: "併合されたテキストの後ろにも和文直後の URL がある",
			src:  "（https://a.example.com/x_y）と（https://b.example.com/）",
			want: `<p>（<a href="https://a.example.com/x_y">https://a.example.com/x_y</a>）と（<a href="https://b.example.com/">https://b.example.com/</a>）</p>`,
		},
		{
			name: "英字の直後はリンクにしない",
			src:  "xhttps://example.com",
			want: `<p>xhttps://example.com</p>`,
		},
		{
			name: "インラインコード内はリンクにしない",
			src:  "`（https://example.com）`",
			want: "<p><code>（https://example.com）</code></p>",
		},
		{
			name: "既存のリンク記法はそのまま",
			src:  "[（https://example.com）](https://example.org)",
			want: `<p><a href="https://example.org">（https://example.com）</a></p>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.TrimSpace(ToHTML(tt.src, Options{}))
			if got != tt.want {
				t.Errorf("ToHTML(%q)\n got: %s\nwant: %s", tt.src, got, tt.want)
			}
		})
	}
}

package markdown

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/osak/mini-nikki/internal/linkpreview"
)

func TestLinkEmbedParse(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{name: "card", src: "@[card](https://example.com/a)", want: []string{"https://example.com/a"}},
		{name: "embed", src: "@[embed](https://youtu.be/dQw4w9WgXcQ)", want: []string{"https://youtu.be/dQw4w9WgXcQ"}},
		{name: "前後の空白", src: "  @[card](https://example.com/a)  ", want: []string{"https://example.com/a"}},
		{
			name: "段落の途中の行",
			src:  "今日読んだ記事\n@[card](https://example.com/a)\nよかった",
			want: []string{"https://example.com/a"},
		},
		{
			name: "重複は 1 つにまとめる",
			src:  "@[card](https://example.com/a)\n\n@[embed](https://example.com/b)\n\n@[card](https://example.com/a)",
			want: []string{"https://example.com/a", "https://example.com/b"},
		},
		{name: "文中は対象外", src: "これ @[card](https://example.com/a) を見て", want: nil},
		{name: "未知の種類", src: "@[video](https://example.com/a)", want: nil},
		{name: "http(s) 以外", src: "@[card](javascript:alert(1))", want: nil},
		{name: "コードブロック内は対象外", src: "```\n@[card](https://example.com/a)\n```", want: nil},
		{name: "インデントコード内は対象外", src: "    @[card](https://example.com/a)", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractEmbedURLs(tt.src)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractEmbedURLs(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

func TestLinkEmbedRenderCard(t *testing.T) {
	const u = "https://example.com/article?a=1&b=2"
	src := "@[card](" + u + ")"

	t.Run("未取得ならフォールバック", func(t *testing.T) {
		got := ToHTML(src, Options{})
		for _, want := range []string{
			`class="link-card link-card-fallback"`,
			`href="https://example.com/article?a=1&amp;b=2"`,
			`<span class="link-card-title">https://example.com/article?a=1&amp;b=2</span>`,
			`<span class="link-card-site">example.com</span>`,
		} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in\n%s", want, got)
			}
		}
		if strings.Contains(got, "<img") {
			t.Errorf("fallback should not have image:\n%s", got)
		}
	})

	t.Run("取得済みならカード", func(t *testing.T) {
		got := ToHTML(src, Options{Previews: map[string]linkpreview.Preview{
			u: {
				URL:         u,
				Title:       "<b>記事</b>のタイトル",
				Description: "説明文",
				ImageURL:    "https://example.com/og.png",
				SiteName:    "Example",
			},
		}})
		for _, want := range []string{
			`<div class="link-card" data-url=`,
			`<span class="link-card-title">&lt;b&gt;記事&lt;/b&gt;のタイトル</span>`,
			`<span class="link-card-description">説明文</span>`,
			`<span class="link-card-site">Example</span>`,
			`<img class="link-card-image" src="https://example.com/og.png"`,
		} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in\n%s", want, got)
			}
		}
	})

	t.Run("フィード向けはただのリンク", func(t *testing.T) {
		got := ToHTML(src, Options{Plain: true, Previews: map[string]linkpreview.Preview{
			u: {URL: u, Title: "記事のタイトル"},
		}})
		want := `<p><a href="https://example.com/article?a=1&amp;b=2">記事のタイトル</a></p>`
		if strings.TrimSpace(got) != want {
			t.Errorf("got %s, want %s", got, want)
		}
	})
}

func TestLinkEmbedRenderInParagraph(t *testing.T) {
	got := ToHTML("前の行\n@[card](https://example.com/)\n後の行", Options{})
	if !strings.HasPrefix(got, "<p>前の行</p>\n<div class=\"link-card") {
		t.Errorf("card should split the paragraph:\n%s", got)
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "<p>後の行</p>") {
		t.Errorf("text after card should be a paragraph:\n%s", got)
	}
}

func TestLinkEmbedRenderEmbed(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "YouTube",
			url:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=10",
			want: `<iframe src="https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"`,
		},
		{
			name: "X",
			url:  "https://x.com/osak/status/1234567890",
			want: `<blockquote class="twitter-tweet" data-dnt="true"><a href="https://twitter.com/osak/status/1234567890">`,
		},
		{
			name: "未対応サイトはカード",
			url:  "https://example.com/video",
			want: `class="link-card link-card-fallback"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToHTML("@[embed]("+tt.url+")", Options{})
			if !strings.Contains(got, tt.want) {
				t.Errorf("missing %q in\n%s", tt.want, got)
			}
		})
	}

	t.Run("フィード向けはただのリンク", func(t *testing.T) {
		got := ToHTML("@[embed](https://youtu.be/dQw4w9WgXcQ)", Options{Plain: true})
		if strings.Contains(got, "<iframe") {
			t.Errorf("feed should not have iframe:\n%s", got)
		}
	})
}

func TestProviderEmbed(t *testing.T) {
	tests := []struct {
		url     string
		youtube string
		tweet   string
	}{
		{url: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", youtube: "dQw4w9WgXcQ"},
		{url: "https://m.youtube.com/watch?v=dQw4w9WgXcQ", youtube: "dQw4w9WgXcQ"},
		{url: "https://youtu.be/dQw4w9WgXcQ?si=abc", youtube: "dQw4w9WgXcQ"},
		{url: "https://www.youtube.com/shorts/dQw4w9WgXcQ", youtube: "dQw4w9WgXcQ"},
		{url: "https://www.youtube.com/embed/dQw4w9WgXcQ", youtube: "dQw4w9WgXcQ"},
		{url: "https://www.youtube.com/watch?v=short"},
		{url: "https://www.youtube.com/watch?v=dQw4w9WgXcQ%22onload"},
		{url: "https://www.youtube.com/channel/UCxxxx"},
		{url: "https://x.com/osak/status/1234567890", tweet: "osak/1234567890"},
		{url: "https://twitter.com/osak/status/1234567890/photo/1", tweet: "osak/1234567890"},
		{url: "https://mobile.twitter.com/osak/status/1234567890", tweet: "osak/1234567890"},
		{url: "https://x.com/osak"},
		{url: "https://x.com/osak/status/abc"},
		{url: "https://example.com/watch?v=dQw4w9WgXcQ"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got, ok := renderProviderEmbed(t, tt.url)
			switch {
			case tt.youtube != "":
				if !ok || !strings.Contains(got, "/embed/"+tt.youtube+`"`) {
					t.Errorf("want YouTube %s, got %q (ok=%v)", tt.youtube, got, ok)
				}
			case tt.tweet != "":
				user, id, _ := strings.Cut(tt.tweet, "/")
				if !ok || !strings.Contains(got, "twitter.com/"+user+"/status/"+id+`"`) {
					t.Errorf("want tweet %s, got %q (ok=%v)", tt.tweet, got, ok)
				}
			default:
				if ok {
					t.Errorf("want no embed, got %q", got)
				}
			}
		})
	}
}

// renderProviderEmbed は providerEmbed の結果を HTML 文字列にする。
func renderProviderEmbed(t *testing.T, rawURL string) (string, bool) {
	t.Helper()
	c, ok := providerEmbed(rawURL)
	if !ok {
		return "", false
	}
	var sb strings.Builder
	if err := c.Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	return sb.String(), true
}

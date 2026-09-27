// Package linkpreview は URL のリンクカード情報（OGP）を取得する。
//
// 取得は投稿の保存時や表示時にバックグラウンドの Worker で行い、結果は DB に
// キャッシュする。ページの描画はキャッシュを読むだけなので、外部サイトの応答速度に
// 表示が律速されない。
package linkpreview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/html/charset"
)

// Preview はリンクカードの表示に使う情報。
type Preview struct {
	URL         string
	Title       string
	Description string
	ImageURL    string
	SiteName    string
}

const (
	fetchTimeout   = 5 * time.Second
	maxRedirects   = 3
	maxBodyBytes   = 1 << 20 // 1 MB。OGP は <head> にあるので十分。
	maxTitleRunes  = 200
	maxDescRunes   = 300
	userAgent      = "Mozilla/5.0 (compatible; mini-nikki-linkpreview/1.0; +https://github.com/osak/mini-nikki)"
	acceptLanguage = "ja,en;q=0.8"
)

// ErrNotHTML は取得先が HTML ではなかったときに返る。
var ErrNotHTML = errors.New("linkpreview: response is not html")

// errPrivateAddress はプライベートアドレスへの接続を拒否したときに返る。
var errPrivateAddress = errors.New("linkpreview: refusing to connect to private address")

// Fetcher は OGP を取得する HTTP クライアント。
type Fetcher struct {
	client *http.Client
}

// NewFetcher は外部サイト向けの Fetcher を作る。
// ループバックやプライベートアドレスへの接続は拒否する。
func NewFetcher() *Fetcher {
	return newFetcher(false)
}

// newFetcher は allowPrivate を true にするとプライベートアドレスへの接続を許可する（テスト用）。
func newFetcher(allowPrivate bool) *Fetcher {
	dialer := &net.Dialer{Timeout: fetchTimeout}
	if !allowPrivate {
		dialer.Control = rejectPrivateAddress
	}
	transport := &http.Transport{
		// プロキシを経由すると接続先アドレスの検査が効かなくなるので使わない。
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   fetchTimeout,
		ResponseHeaderTimeout: fetchTimeout,
	}
	return &Fetcher{client: &http.Client{
		Timeout:   fetchTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("linkpreview: stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}}
}

// rejectPrivateAddress は DNS 解決後の接続先がプライベートアドレスなら接続を拒否する。
// 投稿者は管理者だけだが、サーバーから内部ネットワークを叩けないようにしておく。
func rejectPrivateAddress(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return errPrivateAddress
	}
	return nil
}

// Fetch は rawURL のページを取得して OGP を読む。
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (Preview, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Preview{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", acceptLanguage)

	resp, err := f.client.Do(req)
	if err != nil {
		return Preview{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Preview{}, fmt.Errorf("linkpreview: unexpected status %d", resp.StatusCode)
	}
	contentType := resp.Header.Get("Content-Type")
	if mt, _, err := mime.ParseMediaType(contentType); err == nil &&
		mt != "text/html" && mt != "application/xhtml+xml" {
		return Preview{}, ErrNotHTML
	}

	// Shift_JIS や EUC-JP のページもあるので、Content-Type と <meta charset> から文字コードを判定する。
	body, err := charset.NewReader(io.LimitReader(resp.Body, maxBodyBytes), contentType)
	if err != nil {
		return Preview{}, err
	}
	p := parseHTML(body, resp.Request.URL)
	p.URL = rawURL
	return p, nil
}

// parseHTML は HTML の <head> からリンクカードの情報を読む。
// og:* を優先し、無ければ twitter:* 、<title> / <meta name="description"> の順に使う。
func parseHTML(r io.Reader, base *url.URL) Preview {
	meta := map[string]string{}
	var title string

	z := html.NewTokenizer(r)
	inTitle := false
loop:
	for {
		switch z.Next() {
		case html.ErrorToken:
			break loop
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			switch tok.DataAtom {
			case atom.Meta:
				key, content := metaKeyContent(tok)
				if key != "" {
					if _, ok := meta[key]; !ok {
						meta[key] = content
					}
				}
			case atom.Title:
				inTitle = title == ""
			case atom.Body:
				// OGP は <head> にしか書かれない。本文を読む必要はない。
				break loop
			}
		case html.TextToken:
			if inTitle {
				title += string(z.Text())
			}
		case html.EndTagToken:
			tok := z.Token()
			switch tok.DataAtom {
			case atom.Title:
				inTitle = false
			case atom.Head:
				break loop
			}
		}
	}

	first := func(keys ...string) string {
		for _, k := range keys {
			if v := cleanText(meta[k]); v != "" {
				return v
			}
		}
		return ""
	}

	p := Preview{
		Title:       first("og:title", "twitter:title"),
		Description: first("og:description", "twitter:description", "description"),
		SiteName:    first("og:site_name"),
	}
	if p.Title == "" {
		p.Title = cleanText(title)
	}
	p.Title = truncate(p.Title, maxTitleRunes)
	p.Description = truncate(p.Description, maxDescRunes)
	if img := first("og:image", "og:image:url", "og:image:secure_url", "twitter:image", "twitter:image:src"); img != "" {
		p.ImageURL = resolveHTTPURL(base, img)
	}
	return p
}

// metaKeyContent は <meta property="og:title" content="..."> や
// <meta name="description" content="..."> から (キー, 値) を取り出す。
func metaKeyContent(tok html.Token) (key, content string) {
	for _, a := range tok.Attr {
		switch strings.ToLower(a.Key) {
		case "property", "name":
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(a.Val))
			}
		case "content":
			content = a.Val
		}
	}
	return key, content
}

// resolveHTTPURL は ref を base に対して解決し、http(s) の絶対 URL だけを返す。
func resolveHTTPURL(base *url.URL, ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.String()
}

// cleanText は前後の空白を除き、連続する空白（改行を含む）を 1 つにまとめる。
func cleanText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

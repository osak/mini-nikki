package markdown

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/a-h/templ"
)

// 埋め込みは許可リストに載せたサイトだけ、マークアップを自前で組み立てる。
// oEmbed の html をそのまま差し込むと、外部サイト由来の HTML がページに入り
// XSS の余地が生まれるため使わない。

var (
	youTubeIDRegexp = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	tweetIDRegexp   = regexp.MustCompile(`^[0-9]{1,20}$`)
	xUserRegexp     = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)
)

// providerEmbed は rawURL が対応サイトなら埋め込み用のコンポーネントを返す。
func providerEmbed(rawURL string) (templ.Component, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, false
	}
	if id, ok := youTubeVideoID(u); ok {
		return youTubeEmbed(id), true
	}
	if user, id, ok := tweetID(u); ok {
		return tweetEmbed(user, id, rawURL), true
	}
	return nil, false
}

// youTubeVideoID は YouTube の動画 URL から動画 ID を取り出す。
// 対応形式: youtube.com/watch?v=ID, youtu.be/ID, youtube.com/shorts/ID, youtube.com/embed/ID
func youTubeVideoID(u *url.URL) (string, bool) {
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	host = strings.TrimPrefix(host, "m.")
	var id string
	switch host {
	case "youtube.com", "music.youtube.com":
		switch {
		case u.Path == "/watch":
			id = u.Query().Get("v")
		case strings.HasPrefix(u.Path, "/shorts/"):
			id = strings.TrimPrefix(u.Path, "/shorts/")
		case strings.HasPrefix(u.Path, "/embed/"):
			id = strings.TrimPrefix(u.Path, "/embed/")
		}
	case "youtu.be":
		id = strings.TrimPrefix(u.Path, "/")
	}
	id = strings.TrimSuffix(id, "/")
	if !youTubeIDRegexp.MatchString(id) {
		return "", false
	}
	return id, true
}

// tweetID は X（Twitter）のポスト URL からユーザー名とポスト ID を取り出す。
// 対応形式: x.com/USER/status/ID, twitter.com/USER/status/ID
func tweetID(u *url.URL) (user, id string, ok bool) {
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	host = strings.TrimPrefix(host, "mobile.")
	if host != "x.com" && host != "twitter.com" {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 || parts[1] != "status" {
		return "", "", false
	}
	if !xUserRegexp.MatchString(parts[0]) || !tweetIDRegexp.MatchString(parts[2]) {
		return "", "", false
	}
	return parts[0], parts[2], true
}

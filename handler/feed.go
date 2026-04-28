package handler

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

	"github.com/osak/mini-nikki/internal/markdown"
	"github.com/osak/mini-nikki/model"
)

const siteTitle = "ミニ日記（ゴママヨ）"

// FeedHandler serves RSS 2.0 and Atom 1.0 feeds.
type FeedHandler struct {
	model *model.PostModel
}

func NewFeedHandler(m *model.PostModel) *FeedHandler {
	return &FeedHandler{model: m}
}

// feedBaseURL derives the site base URL from the request.
// X-Forwarded-Proto is honoured for Caddy reverse-proxy deployments.
func feedBaseURL(r *http.Request) string {
	scheme := "https"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

func postPermalink(base string, p model.Post) string {
	return fmt.Sprintf("%s/posts/%d/%02d#post_%d",
		base, p.CreatedAt.Year(), p.CreatedAt.Month(), p.ID)
}

func postFeedTitle(p model.Post) string {
	return fmt.Sprintf("%d年%d月%d日 %s",
		p.CreatedAt.Year(), int(p.CreatedAt.Month()), p.CreatedAt.Day(),
		p.CreatedAt.Format("15:04"))
}

func latestPostTime(posts []model.Post) time.Time {
	var t time.Time
	for _, p := range posts {
		if p.CreatedAt.After(t) {
			t = p.CreatedAt
		}
	}
	return t
}

// ---- RSS 2.0 ---------------------------------------------------------------

type rssRoot struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	LastBuildDate string    `xml:"lastBuildDate,omitempty"`
	Items         []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	GUID        rssGUID `xml:"guid"`
	PubDate     string  `xml:"pubDate"`
	Description string  `xml:"description"`
}

type rssGUID struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

func (h *FeedHandler) RSS(w http.ResponseWriter, r *http.Request) {
	posts, err := h.model.List(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}

	base := feedBaseURL(r)
	items := make([]rssItem, len(posts))
	for i, p := range posts {
		link := postPermalink(base, p)
		items[i] = rssItem{
			Title:       postFeedTitle(p),
			Link:        link,
			GUID:        rssGUID{IsPermaLink: true, Value: link},
			PubDate:     p.CreatedAt.Format(time.RFC1123Z),
			Description: markdown.ToHTML(p.Body),
		}
	}

	var lastBuild string
	if t := latestPostTime(posts); !t.IsZero() {
		lastBuild = t.Format(time.RFC1123Z)
	}

	feed := rssRoot{
		Version: "2.0",
		Channel: rssChannel{
			Title:         siteTitle,
			Link:          base + "/",
			Description:   siteTitle,
			LastBuildDate: lastBuild,
			Items:         items,
		},
	}

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Write([]byte(xml.Header)) //nolint:errcheck
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	enc.Encode(feed) //nolint:errcheck
}

// ---- Atom 1.0 --------------------------------------------------------------

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	XMLNS   string      `xml:"xmlns,attr"`
	Title   string      `xml:"title"`
	Links   []atomLink  `xml:"link"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr,omitempty"`
	Href string `xml:"href,attr"`
	Type string `xml:"type,attr,omitempty"`
}

type atomEntry struct {
	Title   string      `xml:"title"`
	Link    atomLink    `xml:"link"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Content atomContent `xml:"content"`
}

type atomContent struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

func (h *FeedHandler) Atom(w http.ResponseWriter, r *http.Request) {
	posts, err := h.model.List(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}

	base := feedBaseURL(r)
	selfURL := base + "/feed.atom"

	entries := make([]atomEntry, len(posts))
	for i, p := range posts {
		link := postPermalink(base, p)
		entries[i] = atomEntry{
			Title:   postFeedTitle(p),
			Link:    atomLink{Href: link},
			ID:      link,
			Updated: p.CreatedAt.Format(time.RFC3339),
			Content: atomContent{Type: "html", Value: markdown.ToHTML(p.Body)},
		}
	}

	updated := time.Now().Format(time.RFC3339)
	if t := latestPostTime(posts); !t.IsZero() {
		updated = t.Format(time.RFC3339)
	}

	feed := atomFeed{
		XMLNS: "http://www.w3.org/2005/Atom",
		Title: siteTitle,
		Links: []atomLink{
			{Href: base + "/"},
			{Rel: "self", Href: selfURL, Type: "application/atom+xml"},
		},
		ID:      base + "/",
		Updated: updated,
		Entries: entries,
	}

	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.Write([]byte(xml.Header)) //nolint:errcheck
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	enc.Encode(feed) //nolint:errcheck
}

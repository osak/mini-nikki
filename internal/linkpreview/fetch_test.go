package linkpreview

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/text/encoding/japanese"
)

func serve(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchOGP(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html><head>
<title>タイトルタグ</title>
<meta property="og:title" content="  OGP の
  タイトル ">
<meta property="og:description" content="OGP の説明">
<meta property="og:site_name" content="サイト名">
<meta property="og:image" content="/img/og.png">
<meta name="description" content="使われない説明">
</head><body><meta property="og:title" content="body 内は読まない"></body></html>`))
	})

	p, err := newFetcher(true).Fetch(context.Background(), srv.URL+"/article")
	if err != nil {
		t.Fatal(err)
	}
	want := Preview{
		URL:         srv.URL + "/article",
		Title:       "OGP の タイトル",
		Description: "OGP の説明",
		SiteName:    "サイト名",
		ImageURL:    srv.URL + "/img/og.png",
	}
	if p != want {
		t.Errorf("got %+v\nwant %+v", p, want)
	}
}

func TestFetchFallbackToTitleAndDescription(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>
	ページタイトル
</title><meta name="description" content="ページの説明"><meta name="twitter:image" content="https://cdn.example.com/a.png"></head></html>`))
	})

	p, err := newFetcher(true).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "ページタイトル" || p.Description != "ページの説明" || p.ImageURL != "https://cdn.example.com/a.png" {
		t.Errorf("got %+v", p)
	}
}

func TestFetchShiftJIS(t *testing.T) {
	page := `<html><head><meta http-equiv="Content-Type" content="text/html; charset=Shift_JIS">` +
		`<meta property="og:title" content="日本語のタイトル"></head></html>`
	sjis, err := japanese.ShiftJIS.NewEncoder().String(page)
	if err != nil {
		t.Fatal(err)
	}
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		// Content-Type に charset が無く、<meta> でしか分からないケース。
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(sjis))
	})

	p, err := newFetcher(true).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "日本語のタイトル" {
		t.Errorf("title = %q", p.Title)
	}
}

func TestFetchTruncatesLongText(t *testing.T) {
	long := strings.Repeat("あ", 1000)
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<head><meta property="og:title" content="` + long + `"><meta property="og:description" content="` + long + `"></head>`))
	})

	p, err := newFetcher(true).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(p.Title)); n != maxTitleRunes {
		t.Errorf("title length = %d", n)
	}
	if n := len([]rune(p.Description)); n != maxDescRunes {
		t.Errorf("description length = %d", n)
	}
}

func TestFetchRejectsNonHTMLImageURL(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<head><meta property="og:image" content="javascript:alert(1)"></head>`))
	})

	p, err := newFetcher(true).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if p.ImageURL != "" {
		t.Errorf("image url = %q", p.ImageURL)
	}
}

func TestFetchErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{
			name: "404",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.NotFound(w, r)
			},
		},
		{
			name: "HTML 以外",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write([]byte("\x89PNG"))
			},
			wantErr: ErrNotHTML,
		},
		{
			name: "リダイレクトしすぎ",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/loop", http.StatusFound)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := serve(t, tt.handler)
			_, err := newFetcher(true).Fetch(context.Background(), srv.URL)
			if err == nil {
				t.Fatal("want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestFetchRejectsPrivateAddress(t *testing.T) {
	called := false
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	_, err := NewFetcher().Fetch(context.Background(), srv.URL)
	if !errors.Is(err, errPrivateAddress) {
		t.Errorf("err = %v, want %v", err, errPrivateAddress)
	}
	if called {
		t.Error("request reached the loopback server")
	}
}

package model_test

import (
	"context"
	"testing"
	"time"

	"github.com/osak/mini-nikki/internal/linkpreview"
	"github.com/osak/mini-nikki/model"
)

func TestLinkPreview_SaveAndGet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := model.NewLinkPreviewModel(openTestDB(t))

	p := linkpreview.Preview{
		URL: "https://example.com/a", Title: "タイトル", Description: "説明",
		ImageURL: "https://example.com/og.png", SiteName: "Example",
	}
	if err := m.SaveSuccess(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveFailure(ctx, "https://example.com/failed"); err != nil {
		t.Fatal(err)
	}

	got, err := m.GetMany(ctx, []string{"https://example.com/a", "https://example.com/failed", "https://example.com/none"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["https://example.com/a"] != p {
		t.Errorf("GetMany = %+v", got)
	}

	// 上書き
	p.Title = "新しいタイトル"
	if err := m.SaveSuccess(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, _ = m.GetMany(ctx, []string{p.URL})
	if got[p.URL].Title != "新しいタイトル" {
		t.Errorf("title = %q", got[p.URL].Title)
	}

	empty, err := m.GetMany(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("GetMany(nil) = %v, %v", empty, err)
	}
}

func TestLinkPreview_NeedsFetch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := model.NewLinkPreviewModel(openTestDB(t))
	now := time.Unix(1_800_000_000, 0)
	model.SetLinkPreviewNow(m, func() time.Time { return now })

	need := func(url string) bool {
		t.Helper()
		ok, err := m.NeedsFetch(ctx, url, 24*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}

	if !need("https://example.com/none") {
		t.Error("unfetched url should need fetch")
	}

	_ = m.SaveSuccess(ctx, linkpreview.Preview{URL: "https://example.com/ok"})
	if need("https://example.com/ok") {
		t.Error("cached url should not need fetch")
	}

	_ = m.SaveFailure(ctx, "https://example.com/failed")
	now = now.Add(23 * time.Hour)
	if need("https://example.com/failed") {
		t.Error("recently failed url should not need fetch")
	}
	now = now.Add(time.Hour)
	if !need("https://example.com/failed") {
		t.Error("failed url should be retried after 24h")
	}
}

func TestLinkPreview_FailureKeepsPreviousSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := model.NewLinkPreviewModel(openTestDB(t))

	p := linkpreview.Preview{URL: "https://example.com/a", Title: "タイトル"}
	_ = m.SaveSuccess(ctx, p)
	if err := m.SaveFailure(ctx, p.URL); err != nil {
		t.Fatal(err)
	}
	got, _ := m.GetMany(ctx, []string{p.URL})
	if got[p.URL].Title != "タイトル" {
		t.Errorf("previous preview was lost: %+v", got)
	}
}

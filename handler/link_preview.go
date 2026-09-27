package handler

import (
	"context"

	"github.com/osak/mini-nikki/internal/linkpreview"
	"github.com/osak/mini-nikki/internal/markdown"
	"github.com/osak/mini-nikki/model"
)

// LinkPreviews は投稿本文中のリンクカード（@[card] / @[embed]）の OGP を扱う。
//
// 取得はすべてバックグラウンドの Worker が行い、ハンドラはキャッシュを読むことと
// 取得依頼を積むことしかしない。nil のときは何もしない（カードは常にフォールバック表示）。
type LinkPreviews struct {
	model  *model.LinkPreviewModel
	worker *linkpreview.Worker
}

func NewLinkPreviews(m *model.LinkPreviewModel, w *linkpreview.Worker) *LinkPreviews {
	return &LinkPreviews{model: m, worker: w}
}

// Attach は posts の本文中のカード URL について取得済みの OGP を読み、各投稿に設定する。
//
// キャッシュに無い URL は取得キューに積む。既存の投稿のバックフィルや、
// 失敗した URL の再試行もこの経路で行われる。
func (lp *LinkPreviews) Attach(ctx context.Context, posts []model.Post) error {
	if lp == nil {
		return nil
	}
	var urls []string
	seen := map[string]bool{}
	for _, p := range posts {
		for _, u := range markdown.ExtractEmbedURLs(p.Body) {
			if !seen[u] {
				seen[u] = true
				urls = append(urls, u)
			}
		}
	}
	if len(urls) == 0 {
		return nil
	}

	previews, err := lp.model.GetMany(ctx, urls)
	if err != nil {
		return err
	}
	for _, u := range urls {
		if _, ok := previews[u]; !ok {
			lp.worker.Enqueue(u, false)
		}
	}
	for i := range posts {
		posts[i].LinkPreviews = previews
	}
	return nil
}

// Enqueue は body 中のカード URL を取得キューに積む。
// refresh が true ならキャッシュ済みでも取り直す（投稿の編集時）。
func (lp *LinkPreviews) Enqueue(body string, refresh bool) {
	if lp == nil {
		return
	}
	for _, u := range markdown.ExtractEmbedURLs(body) {
		lp.worker.Enqueue(u, refresh)
	}
}

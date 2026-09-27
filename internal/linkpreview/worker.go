package linkpreview

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// RetryAfter は取得に失敗した URL を再試行するまでの間隔。
const RetryAfter = 24 * time.Hour

// queueSize は取得待ちキューの長さ。溢れた分は捨て、次の表示時に積み直される。
const queueSize = 256

// Store は取得結果の保存先。model.LinkPreviewModel が実装する。
type Store interface {
	// NeedsFetch は url を取得すべきかを返す。未取得か、失敗から RetryAfter 以上経っていれば true。
	NeedsFetch(ctx context.Context, url string, retryAfter time.Duration) (bool, error)
	// SaveSuccess は取得に成功した結果を保存する。
	SaveSuccess(ctx context.Context, p Preview) error
	// SaveFailure は url の取得に失敗したことを記録する。前回の成功結果は残す。
	SaveFailure(ctx context.Context, url string) error
}

// fetcher は Fetcher のうち Worker が使う部分。テストで差し替える。
type fetcher interface {
	Fetch(ctx context.Context, url string) (Preview, error)
}

type job struct {
	url     string
	refresh bool
}

// Worker は URL を 1 件ずつ取得して Store に保存するバックグラウンドワーカー。
//
// 外部サイトへのアクセスはすべてここで行い、HTTP リクエストの処理中には行わない
// （Discord Interactions は 3 秒以内に応答する必要がある）。
type Worker struct {
	store   Store
	fetcher fetcher
	queue   chan job

	mu      sync.Mutex
	pending map[string]bool // キューに積まれているか処理中の URL
}

// NewWorker は Worker を作る。取得を始めるには Run を呼ぶ。
func NewWorker(store Store, f *Fetcher) *Worker {
	return newWorker(store, f)
}

func newWorker(store Store, f fetcher) *Worker {
	return &Worker{
		store:   store,
		fetcher: f,
		queue:   make(chan job, queueSize),
		pending: map[string]bool{},
	}
}

// Enqueue は url を取得キューに積む。ブロックしない。
//
// refresh が false のときは、キャッシュ済み（または失敗から RetryAfter 未満）の URL を取得しない。
// true のときはキャッシュの有無にかかわらず取り直す（投稿の編集時に使う）。
func (w *Worker) Enqueue(url string, refresh bool) {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.pending[url] {
		w.mu.Unlock()
		return
	}
	w.pending[url] = true
	w.mu.Unlock()

	select {
	case w.queue <- job{url: url, refresh: refresh}:
	default:
		// キューが満杯。次に表示されたときに積み直されるので捨ててよい。
		w.done(url)
		slog.Warn("linkpreview: queue is full, dropping", "url", url)
	}
}

func (w *Worker) done(url string) {
	w.mu.Lock()
	delete(w.pending, url)
	w.mu.Unlock()
}

// Run は ctx が終了するまでキューを処理する。
func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-w.queue:
			w.process(ctx, j)
			w.done(j.url)
		}
	}
}

func (w *Worker) process(ctx context.Context, j job) {
	if !j.refresh {
		need, err := w.store.NeedsFetch(ctx, j.url, RetryAfter)
		if err != nil {
			slog.Error("linkpreview: failed to check cache", "url", j.url, "err", err)
			return
		}
		if !need {
			return
		}
	}

	p, err := w.fetcher.Fetch(ctx, j.url)
	if err != nil {
		slog.Warn("linkpreview: fetch failed", "url", j.url, "err", err)
		if err := w.store.SaveFailure(ctx, j.url); err != nil {
			slog.Error("linkpreview: failed to save failure", "url", j.url, "err", err)
		}
		return
	}
	if err := w.store.SaveSuccess(ctx, p); err != nil {
		slog.Error("linkpreview: failed to save preview", "url", j.url, "err", err)
		return
	}
	slog.Info("linkpreview: fetched", "url", j.url, "title", p.Title)
}

package linkpreview

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	mu        sync.Mutex
	needFetch map[string]bool
	saved     map[string]Preview
	failed    []string
}

func (s *fakeStore) NeedsFetch(ctx context.Context, url string, retryAfter time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	need, ok := s.needFetch[url]
	return !ok || need, nil
}

func (s *fakeStore) SaveSuccess(ctx context.Context, p Preview) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved[p.URL] = p
	return nil
}

func (s *fakeStore) SaveFailure(ctx context.Context, url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = append(s.failed, url)
	return nil
}

type fakeFetcher struct {
	mu      sync.Mutex
	fetched []string
}

func (f *fakeFetcher) Fetch(ctx context.Context, url string) (Preview, error) {
	f.mu.Lock()
	f.fetched = append(f.fetched, url)
	f.mu.Unlock()
	if url == "https://fail.example.com/" {
		return Preview{}, errors.New("boom")
	}
	return Preview{URL: url, Title: "title of " + url}, nil
}

// drain は Run を動かし、キューが空になるまで待つ。
func drain(t *testing.T, w *Worker) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		n := len(w.pending)
		w.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("worker did not finish")
}

func TestWorker(t *testing.T) {
	store := &fakeStore{
		needFetch: map[string]bool{"https://cached.example.com/": false},
		saved:     map[string]Preview{},
	}
	f := &fakeFetcher{}
	w := newWorker(store, f)

	w.Enqueue("https://new.example.com/", false)
	w.Enqueue("https://new.example.com/", false) // 重複は積まない
	w.Enqueue("https://cached.example.com/", false)
	w.Enqueue("https://fail.example.com/", false)
	drain(t, w)

	if got := f.fetched; len(got) != 2 || got[0] != "https://new.example.com/" || got[1] != "https://fail.example.com/" {
		t.Errorf("fetched = %v", got)
	}
	if store.saved["https://new.example.com/"].Title != "title of https://new.example.com/" {
		t.Errorf("saved = %v", store.saved)
	}
	if len(store.failed) != 1 || store.failed[0] != "https://fail.example.com/" {
		t.Errorf("failed = %v", store.failed)
	}

	// refresh はキャッシュ済みでも取り直す。
	w.Enqueue("https://cached.example.com/", true)
	drain(t, w)
	if _, ok := store.saved["https://cached.example.com/"]; !ok {
		t.Errorf("refresh did not refetch: fetched = %v", f.fetched)
	}
}

func TestWorkerNilIsNoop(t *testing.T) {
	var w *Worker
	w.Enqueue("https://example.com/", false) // panic しない
}

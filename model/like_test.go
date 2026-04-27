package model_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	minidb "github.com/osak/mini-nikki/db"
	"github.com/osak/mini-nikki/model"
)

// openTestDB opens a fresh file-based SQLite DB with migrations applied.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := minidb.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// newPost creates a post and returns its ID.
func newPost(t *testing.T, pm *model.PostModel) int64 {
	t.Helper()
	id, err := pm.Create(context.Background(), "test")
	if err != nil {
		t.Fatalf("create post: %v", err)
	}
	return id
}

// seedLike inserts a like row with the current timestamp (counts as today).
func seedLike(t *testing.T, database *sql.DB, postID int64, ip, cookieID string) {
	t.Helper()
	var c interface{}
	if cookieID != "" {
		c = cookieID
	}
	_, err := database.ExecContext(context.Background(),
		`INSERT INTO likes (post_id, ip_address, cookie_id) VALUES (?, ?, ?)`,
		postID, ip, c,
	)
	if err != nil {
		t.Fatalf("seedLike: %v", err)
	}
}

// ---- no-cookie mode --------------------------------------------------------

func TestNoCookie_FirstLike_Added(t *testing.T) {
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)

	added, err := lm.AddLike(context.Background(), newPost(t, pm), "1.2.3.4", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !added {
		t.Error("want added=true, got false")
	}
}

func TestNoCookie_DuplicatePost_NotAdded(t *testing.T) {
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)
	pid := newPost(t, pm)

	seedLike(t, database, pid, "1.2.3.4", "")

	added, err := lm.AddLike(context.Background(), pid, "1.2.3.4", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added {
		t.Error("want added=false for duplicate, got true")
	}
}

func TestNoCookie_10thLike_Succeeds(t *testing.T) {
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)

	for i := 0; i < 9; i++ {
		seedLike(t, database, newPost(t, pm), "1.2.3.4", "")
	}

	added, err := lm.AddLike(context.Background(), newPost(t, pm), "1.2.3.4", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !added {
		t.Error("want added=true for 10th like, got false")
	}
}

func TestNoCookie_11thLike_RateLimited(t *testing.T) {
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)

	for i := 0; i < 10; i++ {
		seedLike(t, database, newPost(t, pm), "1.2.3.4", "")
	}

	_, err := lm.AddLike(context.Background(), newPost(t, pm), "1.2.3.4", "")
	if !errors.Is(err, model.ErrRateLimited) {
		t.Errorf("want ErrRateLimited, got %v", err)
	}
}

func TestNoCookie_DifferentIPs_Independent(t *testing.T) {
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)

	for i := 0; i < 10; i++ {
		seedLike(t, database, newPost(t, pm), "1.2.3.4", "")
	}

	// 別IPは制限を受けない
	added, err := lm.AddLike(context.Background(), newPost(t, pm), "5.6.7.8", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !added {
		t.Error("want added=true for different IP, got false")
	}
}

// ---- cookie mode -----------------------------------------------------------

func TestCookie_FirstLike_Added(t *testing.T) {
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)

	added, err := lm.AddLike(context.Background(), newPost(t, pm), "1.2.3.4", "cookie-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !added {
		t.Error("want added=true, got false")
	}
}

func TestCookie_10thLikeOnSamePost_Succeeds(t *testing.T) {
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)
	pid := newPost(t, pm)

	for i := 0; i < 9; i++ {
		seedLike(t, database, pid, "1.2.3.4", "cookie-a")
	}

	added, err := lm.AddLike(context.Background(), pid, "1.2.3.4", "cookie-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !added {
		t.Error("want added=true for 10th like on same post, got false")
	}
}

func TestCookie_11thLikeOnSamePost_RateLimited(t *testing.T) {
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)
	pid := newPost(t, pm)

	for i := 0; i < 10; i++ {
		seedLike(t, database, pid, "1.2.3.4", "cookie-a")
	}

	_, err := lm.AddLike(context.Background(), pid, "1.2.3.4", "cookie-a")
	if !errors.Is(err, model.ErrRateLimited) {
		t.Errorf("want ErrRateLimited after 10 likes on same post, got %v", err)
	}
}

func TestCookie_PostLimitIsPerPost(t *testing.T) {
	// 同一Cookieでも別記事なら制限されない
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)
	pidA := newPost(t, pm)
	pidB := newPost(t, pm)

	for i := 0; i < 10; i++ {
		seedLike(t, database, pidA, "1.2.3.4", "cookie-a")
	}

	added, err := lm.AddLike(context.Background(), pidB, "1.2.3.4", "cookie-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !added {
		t.Error("want added=true for different post, got false")
	}
}

func TestCookie_5thDistinctCookieFromIP_Succeeds(t *testing.T) {
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)

	for i := 0; i < 4; i++ {
		seedLike(t, database, newPost(t, pm), "1.2.3.4", fmt.Sprintf("cookie-%d", i))
	}

	added, err := lm.AddLike(context.Background(), newPost(t, pm), "1.2.3.4", "cookie-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !added {
		t.Error("want added=true for 5th distinct cookie, got false")
	}
}

func TestCookie_6thDistinctCookieFromIP_RateLimited(t *testing.T) {
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)

	for i := 0; i < 5; i++ {
		seedLike(t, database, newPost(t, pm), "1.2.3.4", fmt.Sprintf("cookie-%d", i))
	}

	_, err := lm.AddLike(context.Background(), newPost(t, pm), "1.2.3.4", "cookie-new")
	if !errors.Is(err, model.ErrRateLimited) {
		t.Errorf("want ErrRateLimited for 6th distinct cookie from same IP, got %v", err)
	}
}

func TestCookie_ExistingCookieNotBlockedByIPLimit(t *testing.T) {
	// 既に今日このIPでLike済みのCookieは5枠のうち1枠を使っているが、
	// 追加のLikeは引き続き許可される。
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)

	// 4つの別Cookieが既にLike済み
	for i := 0; i < 4; i++ {
		seedLike(t, database, newPost(t, pm), "1.2.3.4", fmt.Sprintf("cookie-%d", i))
	}
	// 自分のCookieも既に1件Like済み（5枠目）
	seedLike(t, database, newPost(t, pm), "1.2.3.4", "cookie-mine")

	// 5枠すべて埋まっているが、自分のCookieはまだ別記事をLikeできる
	added, err := lm.AddLike(context.Background(), newPost(t, pm), "1.2.3.4", "cookie-mine")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !added {
		t.Error("want added=true for existing cookie within 5-slot limit, got false")
	}
}

func TestCookie_NoIPDailyLimit(t *testing.T) {
	// Cookieモードには10件/日のIP制限がない。
	// 同一CookieでもN個の異なる記事をLikeできる（per-post上限10件に達しなければ）。
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)

	for i := 0; i < 10; i++ {
		seedLike(t, database, newPost(t, pm), "1.2.3.4", "cookie-a")
	}

	// 11記事目（per-post limit未達）は成功すべき
	added, err := lm.AddLike(context.Background(), newPost(t, pm), "1.2.3.4", "cookie-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !added {
		t.Error("want added=true: cookie mode has no 10/day IP limit, got false")
	}
}

func TestCookie_DifferentIPsSameLimit(t *testing.T) {
	// IPが異なれば5Cookie/日の枠は独立している
	t.Parallel()
	database := openTestDB(t)
	pm := model.NewPostModel(database)
	lm := model.NewLikeModel(database)

	// IP-A で5枠を使い切る
	for i := 0; i < 5; i++ {
		seedLike(t, database, newPost(t, pm), "1.2.3.4", fmt.Sprintf("cookie-%d", i))
	}

	// IP-B の新Cookieは影響を受けない
	added, err := lm.AddLike(context.Background(), newPost(t, pm), "9.9.9.9", "cookie-x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !added {
		t.Error("want added=true for different IP, got false")
	}
}

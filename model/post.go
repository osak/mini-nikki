package model

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// jst は UTC+9 固定。time.LoadLocation を避けることで tzdata 依存をなくす。
var jst = time.FixedZone("JST", 9*60*60)

// 投稿の由来。posts.source に保存する。
const (
	SourceWeb     = "web"
	SourceDiscord = "discord"
)

// ErrDuplicatePost は同一の Discord メッセージから 2 回目の投稿を試みたときに返る。
var ErrDuplicatePost = errors.New("post already exists for this discord message")

type Post struct {
	ID        int64
	Body      string
	CreatedAt time.Time
	LikeCount int64
	HasLiked  bool
	// Source は SourceWeb または SourceDiscord。
	Source string
	// DiscordMessageID は Discord のメッセージから作られた投稿のみ設定される。
	// スラッシュコマンド経由の投稿や web 投稿では空文字列。
	DiscordMessageID string
}

type PostGroup struct {
	Date  time.Time
	Posts []Post
}

func GroupByDate(posts []Post) []PostGroup {
	var groups []PostGroup
	for _, p := range posts {
		y, m, d := p.CreatedAt.Date()
		date := time.Date(y, m, d, 0, 0, 0, 0, jst)
		if len(groups) == 0 || !groups[len(groups)-1].Date.Equal(date) {
			groups = append(groups, PostGroup{Date: date})
		}
		groups[len(groups)-1].Posts = append(groups[len(groups)-1].Posts, p)
	}
	return groups
}

type PostModel struct {
	db *sql.DB
}

func NewPostModel(db *sql.DB) *PostModel {
	return &PostModel{db: db}
}

func toJST(epoch int64) time.Time {
	return time.Unix(epoch, 0).In(jst)
}

// postColumns は scanPost が期待する SELECT 句。
const postColumns = `id, body, created_at, source, COALESCE(discord_message_id, '')`

// rowScanner は *sql.Row と *sql.Rows の共通部分。
type rowScanner interface {
	Scan(dest ...any) error
}

func scanPost(s rowScanner) (Post, error) {
	var p Post
	var epoch int64
	if err := s.Scan(&p.ID, &p.Body, &epoch, &p.Source, &p.DiscordMessageID); err != nil {
		return Post{}, err
	}
	p.CreatedAt = toJST(epoch)
	return p, nil
}

func scanPosts(rows *sql.Rows) ([]Post, error) {
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

func (m *PostModel) List(ctx context.Context) ([]Post, error) {
	// JST日付降順、同日内は時刻昇順。
	// (created_at + 32400) / 86400 で JST の日番号を得る（32400 = 9*3600）。
	rows, err := m.db.QueryContext(ctx,
		`SELECT `+postColumns+` FROM posts
		 ORDER BY (created_at + 32400) / 86400 DESC, created_at ASC
		 LIMIT 20`)
	if err != nil {
		return nil, err
	}
	return scanPosts(rows)
}

func (m *PostModel) ListByMonth(ctx context.Context, year, month int) ([]Post, error) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, jst)
	end := start.AddDate(0, 1, 0)
	rows, err := m.db.QueryContext(ctx,
		`SELECT `+postColumns+` FROM posts
		 WHERE created_at >= ? AND created_at < ?
		 ORDER BY (created_at + 32400) / 86400 DESC, created_at ASC`,
		start.Unix(),
		end.Unix(),
	)
	if err != nil {
		return nil, err
	}
	return scanPosts(rows)
}

func (m *PostModel) Get(ctx context.Context, id int64) (Post, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+postColumns+` FROM posts WHERE id = ?`, id)
	return scanPost(row)
}

// Create は管理画面からの投稿を作成する。
func (m *PostModel) Create(ctx context.Context, body string) (int64, error) {
	return m.insert(ctx, body, SourceWeb, "")
}

// CreateFromDiscord は Discord 由来の投稿を作成する。
//
// messageID には元となった Discord メッセージの ID を渡す。同じ ID で 2 回目の
// 呼び出しを行うと ErrDuplicatePost が返る（メッセージコマンドの二度押しや
// Discord 側の再送でも記事が重複しない）。元メッセージが存在しない
// スラッシュコマンド経由の投稿では空文字列を渡す。
func (m *PostModel) CreateFromDiscord(ctx context.Context, body, messageID string) (int64, error) {
	return m.insert(ctx, body, SourceDiscord, messageID)
}

func (m *PostModel) insert(ctx context.Context, body, source, messageID string) (int64, error) {
	// 部分 UNIQUE インデックスは NULL を無視するので、ID なしは NULL で入れる。
	var mid any
	if messageID != "" {
		mid = messageID
	}

	res, err := m.db.ExecContext(ctx,
		`INSERT INTO posts (body, created_at, source, discord_message_id) VALUES (?, ?, ?, ?)`,
		body, time.Now().Unix(), source, mid)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrDuplicatePost
		}
		return 0, err
	}
	return res.LastInsertId()
}

// isUniqueViolation は UNIQUE 制約違反かどうかを判定する。
// modernc.org/sqlite はドライバ固有のエラー型を公開していないためメッセージで判定する。
func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func (m *PostModel) Update(ctx context.Context, id int64, body string) error {
	_, err := m.db.ExecContext(ctx,
		`UPDATE posts SET body = ? WHERE id = ?`, body, id)
	return err
}

func (m *PostModel) Delete(ctx context.Context, id int64) error {
	_, err := m.db.ExecContext(ctx,
		`DELETE FROM posts WHERE id = ?`, id)
	return err
}

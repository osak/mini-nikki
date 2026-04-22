package model

import (
	"context"
	"database/sql"
	"time"
)

// jst は UTC+9 固定。time.LoadLocation を避けることで tzdata 依存をなくす。
var jst = time.FixedZone("JST", 9*60*60)

type Post struct {
	ID        int64
	Body      string
	CreatedAt time.Time
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

func (m *PostModel) List(ctx context.Context) ([]Post, error) {
	// JST日付降順、同日内は時刻昇順。
	// (created_at + 32400) / 86400 で JST の日番号を得る（32400 = 9*3600）。
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, body, created_at FROM posts
		 ORDER BY (created_at + 32400) / 86400 DESC, created_at ASC
		 LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		var p Post
		var epoch int64
		if err := rows.Scan(&p.ID, &p.Body, &epoch); err != nil {
			return nil, err
		}
		p.CreatedAt = toJST(epoch)
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

func (m *PostModel) ListByMonth(ctx context.Context, year, month int) ([]Post, error) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, jst)
	end := start.AddDate(0, 1, 0)
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, body, created_at FROM posts
		 WHERE created_at >= ? AND created_at < ?
		 ORDER BY (created_at + 32400) / 86400 DESC, created_at ASC`,
		start.Unix(),
		end.Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		var p Post
		var epoch int64
		if err := rows.Scan(&p.ID, &p.Body, &epoch); err != nil {
			return nil, err
		}
		p.CreatedAt = toJST(epoch)
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

func (m *PostModel) Get(ctx context.Context, id int64) (Post, error) {
	var p Post
	var epoch int64
	err := m.db.QueryRowContext(ctx,
		`SELECT id, body, created_at FROM posts WHERE id = ?`, id).
		Scan(&p.ID, &p.Body, &epoch)
	if err != nil {
		return Post{}, err
	}
	p.CreatedAt = toJST(epoch)
	return p, err
}

func (m *PostModel) Create(ctx context.Context, body string) (int64, error) {
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO posts (body, created_at) VALUES (?, ?)`,
		body, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *PostModel) Delete(ctx context.Context, id int64) error {
	_, err := m.db.ExecContext(ctx,
		`DELETE FROM posts WHERE id = ?`, id)
	return err
}

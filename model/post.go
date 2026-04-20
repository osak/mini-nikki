package model

import (
	"context"
	"database/sql"
	"time"
)

type Post struct {
	ID        int64
	Body      string
	CreatedAt time.Time
}

type PostModel struct {
	db *sql.DB
}

func NewPostModel(db *sql.DB) *PostModel {
	return &PostModel{db: db}
}

func (m *PostModel) List(ctx context.Context) ([]Post, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, body, created_at FROM posts ORDER BY created_at DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Body, &p.CreatedAt); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

func (m *PostModel) Get(ctx context.Context, id int64) (Post, error) {
	var p Post
	err := m.db.QueryRowContext(ctx,
		`SELECT id, body, created_at FROM posts WHERE id = ?`, id).
		Scan(&p.ID, &p.Body, &p.CreatedAt)
	return p, err
}

func (m *PostModel) Create(ctx context.Context, body string) (int64, error) {
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO posts (body) VALUES (?)`, body)
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

package model

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/osak/mini-nikki/internal/linkpreview"
)

// link_previews.status の値。
const (
	previewStatusOK     = "ok"
	previewStatusFailed = "failed"
)

// LinkPreviewModel はリンクカード用の OGP キャッシュ（link_previews テーブル）を扱う。
// linkpreview.Store を実装する。
type LinkPreviewModel struct {
	db  *sql.DB
	now func() time.Time
}

func NewLinkPreviewModel(db *sql.DB) *LinkPreviewModel {
	return &LinkPreviewModel{db: db, now: time.Now}
}

var _ linkpreview.Store = (*LinkPreviewModel)(nil)

// GetMany は urls のうち取得に成功しているものを URL をキーにして返す。
func (m *LinkPreviewModel) GetMany(ctx context.Context, urls []string) (map[string]linkpreview.Preview, error) {
	result := map[string]linkpreview.Preview{}
	if len(urls) == 0 {
		return result, nil
	}

	args := make([]any, 0, len(urls)+1)
	args = append(args, previewStatusOK)
	for _, u := range urls {
		args = append(args, u)
	}
	placeholders := strings.Repeat(",?", len(urls))[1:]

	rows, err := m.db.QueryContext(ctx,
		`SELECT url, title, description, image_url, site_name FROM link_previews
		 WHERE status = ? AND url IN (`+placeholders+`)`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var p linkpreview.Preview
		if err := rows.Scan(&p.URL, &p.Title, &p.Description, &p.ImageURL, &p.SiteName); err != nil {
			return nil, err
		}
		result[p.URL] = p
	}
	return result, rows.Err()
}

// NeedsFetch は url が未取得か、取得失敗から retryAfter 以上経っていれば true を返す。
func (m *LinkPreviewModel) NeedsFetch(ctx context.Context, url string, retryAfter time.Duration) (bool, error) {
	var status string
	var fetchedAt int64
	err := m.db.QueryRowContext(ctx,
		`SELECT status, fetched_at FROM link_previews WHERE url = ?`, url,
	).Scan(&status, &fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if status == previewStatusOK {
		return false, nil
	}
	return m.now().Sub(time.Unix(fetchedAt, 0)) >= retryAfter, nil
}

// SaveSuccess は取得結果を保存する。既存の行は上書きする。
func (m *LinkPreviewModel) SaveSuccess(ctx context.Context, p linkpreview.Preview) error {
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO link_previews (url, status, title, description, image_url, site_name, fetched_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(url) DO UPDATE SET
		     status = excluded.status,
		     title = excluded.title,
		     description = excluded.description,
		     image_url = excluded.image_url,
		     site_name = excluded.site_name,
		     fetched_at = excluded.fetched_at`,
		p.URL, previewStatusOK, p.Title, p.Description, p.ImageURL, p.SiteName, m.now().Unix())
	return err
}

// SaveFailure は url の取得に失敗したことを記録する。
//
// 以前に取得に成功している場合はその結果を残す（一時的な障害でカードが消えないように）。
func (m *LinkPreviewModel) SaveFailure(ctx context.Context, url string) error {
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO link_previews (url, status, fetched_at) VALUES (?, ?, ?)
		 ON CONFLICT(url) DO UPDATE SET fetched_at = excluded.fetched_at
		 WHERE link_previews.status = ?`,
		url, previewStatusFailed, m.now().Unix(), previewStatusFailed)
	return err
}

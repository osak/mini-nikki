package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrRateLimited = errors.New("rate limited")

type LikeModel struct {
	db *sql.DB
}

func NewLikeModel(db *sql.DB) *LikeModel {
	return &LikeModel{db: db}
}

// todayUnix returns the Unix timestamp of the current JST midnight.
func todayUnix() int64 {
	now := time.Now().In(jst)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, jst)
	return today.Unix()
}

// AddLike attempts to record a like for postID from the given IP / cookieID.
//
// Rate limiting rules:
//   - No cookie: same IP can like at most 10 times per day (unique per post).
//   - Cookie present: no IP-based daily limit, but at most 5 distinct cookie
//     IDs from the same IP are allowed to like per day, and the same cookie
//     can like the same post at most 10 times.
//
// Returns (true, nil) on success, (false, nil) if no-cookie duplicate,
// and (false, ErrRateLimited) when the request is blocked.
func (m *LikeModel) AddLike(ctx context.Context, postID int64, ip, cookieID string) (bool, error) {
	today := todayUnix()

	if cookieID != "" {
		// Cookie mode ---------------------------------------------------
		// 1. Check 5-distinct-cookies-per-IP-per-day limit.
		var currentActive int
		if err := m.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM likes
			 WHERE ip_address = ? AND liked_at >= ? AND cookie_id = ?`,
			ip, today, cookieID,
		).Scan(&currentActive); err != nil {
			return false, err
		}
		if currentActive == 0 {
			var distinctCookies int
			if err := m.db.QueryRowContext(ctx,
				`SELECT COUNT(DISTINCT cookie_id) FROM likes
				 WHERE ip_address = ? AND liked_at >= ? AND cookie_id IS NOT NULL`,
				ip, today,
			).Scan(&distinctCookies); err != nil {
				return false, err
			}
			if distinctCookies >= 5 {
				return false, ErrRateLimited
			}
		}

		// 2. Check per-(cookie, post) limit: at most 10 likes allowed.
		var perPost int
		if err := m.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM likes WHERE post_id = ? AND cookie_id = ?`,
			postID, cookieID,
		).Scan(&perPost); err != nil {
			return false, err
		}
		if perPost >= 10 {
			return false, ErrRateLimited
		}

		// Insert without IGNORE — duplicates are intentional here.
		if _, err := m.db.ExecContext(ctx,
			`INSERT INTO likes (post_id, ip_address, cookie_id) VALUES (?, ?, ?)`,
			postID, ip, cookieID,
		); err != nil {
			return false, err
		}
		return true, nil
	}

	// No-cookie mode --------------------------------------------------------
	var count int
	if err := m.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM likes
		 WHERE ip_address = ? AND liked_at >= ? AND cookie_id IS NULL`,
		ip, today,
	).Scan(&count); err != nil {
		return false, err
	}
	if count >= 10 {
		return false, ErrRateLimited
	}

	// UNIQUE index on (post_id, ip_address) WHERE cookie_id IS NULL silently
	// prevents double-liking the same post.
	result, err := m.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO likes (post_id, ip_address, cookie_id) VALUES (?, ?, NULL)`,
		postID, ip,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// CountByPost returns the total number of likes for a single post.
func (m *LikeModel) CountByPost(ctx context.Context, postID int64) (int64, error) {
	var count int64
	err := m.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM likes WHERE post_id = ?`, postID,
	).Scan(&count)
	return count, err
}

// EnrichPosts populates LikeCount and HasLiked on each Post in the slice.
// The slice elements are modified in place.
func (m *LikeModel) EnrichPosts(ctx context.Context, posts []Post, ip, cookieID string) error {
	if len(posts) == 0 {
		return nil
	}

	ids := make([]interface{}, len(posts))
	for i, p := range posts {
		ids[i] = p.ID
	}
	placeholders := strings.Join(strings.Split(strings.Repeat("?", len(ids)), ""), ",")
	inClause := fmt.Sprintf("(%s)", placeholders)

	// --- like counts ---
	countRows, err := m.db.QueryContext(ctx,
		`SELECT post_id, COUNT(*) FROM likes WHERE post_id IN `+inClause+` GROUP BY post_id`,
		ids...,
	)
	if err != nil {
		return err
	}
	counts := make(map[int64]int64, len(posts))
	for countRows.Next() {
		var pid, cnt int64
		if err := countRows.Scan(&pid, &cnt); err != nil {
			countRows.Close()
			return err
		}
		counts[pid] = cnt
	}
	countRows.Close()
	if err := countRows.Err(); err != nil {
		return err
	}

	// --- HasLiked ---
	likedSet := make(map[int64]bool, len(posts))
	var likedRows *sql.Rows
	if cookieID != "" {
		args := append(ids, cookieID)
		likedRows, err = m.db.QueryContext(ctx,
			`SELECT post_id FROM likes WHERE post_id IN `+inClause+` AND cookie_id = ?`,
			args...,
		)
	} else {
		args := append(ids, ip)
		likedRows, err = m.db.QueryContext(ctx,
			`SELECT post_id FROM likes WHERE post_id IN `+inClause+` AND ip_address = ? AND cookie_id IS NULL`,
			args...,
		)
	}
	if err != nil {
		return err
	}
	for likedRows.Next() {
		var pid int64
		if err := likedRows.Scan(&pid); err != nil {
			likedRows.Close()
			return err
		}
		likedSet[pid] = true
	}
	likedRows.Close()
	if err := likedRows.Err(); err != nil {
		return err
	}

	for i := range posts {
		posts[i].LikeCount = counts[posts[i].ID]
		posts[i].HasLiked = likedSet[posts[i].ID]
	}
	return nil
}

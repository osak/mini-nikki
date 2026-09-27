package templates

import (
	"time"

	"github.com/osak/mini-nikki/internal/markdown"
	"github.com/osak/mini-nikki/model"
)

var weekdays = [...]string{"日", "月", "火", "水", "木", "金", "土"}

func formatDate(t time.Time) string {
	return t.Format("2006年1月2日") + "（" + weekdays[t.Weekday()] + "）"
}

func postBodyHTML(post model.Post) string {
	return markdown.ToHTML(post.Body, markdown.Options{Previews: post.LinkPreviews})
}

package templates

import (
	"bytes"
	"html"
	"time"

	"github.com/yuin/goldmark"
)

var weekdays = [...]string{"日", "月", "火", "水", "木", "金", "土"}

func formatDate(t time.Time) string {
	return t.Format("2006年1月2日") + "（" + weekdays[t.Weekday()] + "）"
}

func markdownToHTML(src string) string {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(src), &buf); err != nil {
		return html.EscapeString(src)
	}
	return buf.String()
}

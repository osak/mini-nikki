package templates

import "time"

var weekdays = [...]string{"日", "月", "火", "水", "木", "金", "土"}

func formatDate(t time.Time) string {
	return t.Format("2006年1月2日") + "（" + weekdays[t.Weekday()] + "）"
}

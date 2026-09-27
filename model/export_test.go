package model

import "time"

// SetLinkPreviewNow はテストから LinkPreviewModel の現在時刻を差し替える。
func SetLinkPreviewNow(m *LinkPreviewModel, now func() time.Time) {
	m.now = now
}

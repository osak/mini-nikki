package handler_test

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	minidb "github.com/osak/mini-nikki/db"
	"github.com/osak/mini-nikki/handler"
	"github.com/osak/mini-nikki/model"
)

const allowedUser = "111111111111111111"

// newTestHandler returns a DiscordHandler backed by a fresh SQLite DB, along
// with the private key Discord would sign requests with and the post model.
func newTestHandler(t *testing.T) (*handler.DiscordHandler, ed25519.PrivateKey, *model.PostModel) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	database, err := minidb.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	pm := model.NewPostModel(database)
	h, err := handler.NewDiscordHandler(pm, hex.EncodeToString(pub), []string{allowedUser})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	return h, priv, pm
}

// post signs body the way Discord does and runs it through the handler.
func post(t *testing.T, h *handler.DiscordHandler, priv ed25519.PrivateKey, body string) *httptest.ResponseRecorder {
	t.Helper()
	const timestamp = "1700000000"
	sig := ed25519.Sign(priv, []byte(timestamp+body))

	r := httptest.NewRequest(http.MethodPost, "/webhooks/discord", strings.NewReader(body))
	r.Header.Set("X-Signature-Timestamp", timestamp)
	r.Header.Set("X-Signature-Ed25519", hex.EncodeToString(sig))

	w := httptest.NewRecorder()
	h.Interactions(w, r)
	return w
}

// replyContent extracts the ephemeral message text from an interaction response.
func replyContent(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Type int `json:"type"`
		Data struct {
			Content string `json:"content"`
			Flags   int    `json:"flags"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, w.Body.String())
	}
	if resp.Type != 4 {
		t.Fatalf("want response type 4, got %d", resp.Type)
	}
	if resp.Data.Flags&(1<<6) == 0 {
		t.Error("want ephemeral flag set on reply")
	}
	return resp.Data.Content
}

func slashCommand(userID, body string) string {
	return `{"id":"1","type":2,"data":{"name":"nikki","type":1,"options":[{"name":"body","type":3,"value":` +
		mustJSON(body) + `}]},"member":{"user":{"id":"` + userID + `"}}}`
}

func messageCommand(userID, messageID, content string) string {
	return `{"id":"1","type":2,"data":{"name":"日記に投稿","type":3,"target_id":"` + messageID +
		`","resolved":{"messages":{"` + messageID + `":{"id":"` + messageID + `","content":` +
		mustJSON(content) + `}}}},"member":{"user":{"id":"` + userID + `"}}}`
}

func mustJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func listBodies(t *testing.T, pm *model.PostModel) []model.Post {
	t.Helper()
	posts, err := pm.List(context.Background())
	if err != nil {
		t.Fatalf("list posts: %v", err)
	}
	return posts
}

// ---- signature verification ------------------------------------------------

func TestPing_Ponged(t *testing.T) {
	t.Parallel()
	h, priv, _ := newTestHandler(t)

	w := post(t, h, priv, `{"id":"1","type":1}`)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var resp struct {
		Type int `json:"type"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Type != 1 {
		t.Errorf("want PONG (type 1), got %d", resp.Type)
	}
}

func TestBadSignature_401(t *testing.T) {
	// Discord はエンドポイント登録時に不正な署名を送り、401 を期待する。
	t.Parallel()
	h, _, pm := newTestHandler(t)

	_, otherKey, _ := ed25519.GenerateKey(nil)
	w := post(t, h, otherKey, slashCommand(allowedUser, "のっとり"))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for wrong key, got %d", w.Code)
	}
	if got := listBodies(t, pm); len(got) != 0 {
		t.Errorf("want no post created, got %d", len(got))
	}
}

func TestMissingSignatureHeaders_401(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/webhooks/discord", strings.NewReader(`{"type":1}`))
	w := httptest.NewRecorder()
	h.Interactions(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401 without signature headers, got %d", w.Code)
	}
}

func TestTamperedBody_401(t *testing.T) {
	// 署名は timestamp + 生ボディに対するもの。ボディを差し替えたら通らない。
	t.Parallel()
	h, priv, _ := newTestHandler(t)

	const timestamp = "1700000000"
	sig := ed25519.Sign(priv, []byte(timestamp+slashCommand(allowedUser, "もとの本文")))

	r := httptest.NewRequest(http.MethodPost, "/webhooks/discord",
		strings.NewReader(slashCommand(allowedUser, "すりかえた本文")))
	r.Header.Set("X-Signature-Timestamp", timestamp)
	r.Header.Set("X-Signature-Ed25519", hex.EncodeToString(sig))

	w := httptest.NewRecorder()
	h.Interactions(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401 for tampered body, got %d", w.Code)
	}
}

// ---- authorization ---------------------------------------------------------

func TestUnauthorizedUser_NoPost(t *testing.T) {
	t.Parallel()
	h, priv, pm := newTestHandler(t)

	w := post(t, h, priv, slashCommand("999999999999999999", "よそのひと"))

	if got, want := replyContent(t, w), "権限がありません"; !strings.Contains(got, want) {
		t.Errorf("want reply containing %q, got %q", want, got)
	}
	if got := listBodies(t, pm); len(got) != 0 {
		t.Errorf("want no post created, got %d", len(got))
	}
}

// ---- slash command ---------------------------------------------------------

func TestSlashCommand_CreatesPost(t *testing.T) {
	t.Parallel()
	h, priv, pm := newTestHandler(t)

	w := post(t, h, priv, slashCommand(allowedUser, "  今日はいい天気  "))

	if got := replyContent(t, w); !strings.Contains(got, "投稿しました") {
		t.Errorf("want success reply, got %q", got)
	}
	posts := listBodies(t, pm)
	if len(posts) != 1 {
		t.Fatalf("want 1 post, got %d", len(posts))
	}
	if posts[0].Body != "今日はいい天気" {
		t.Errorf("want trimmed body, got %q", posts[0].Body)
	}
	if posts[0].Source != model.SourceDiscord {
		t.Errorf("want source %q, got %q", model.SourceDiscord, posts[0].Source)
	}
	if posts[0].DiscordMessageID != "" {
		t.Errorf("want empty message ID for slash command, got %q", posts[0].DiscordMessageID)
	}
}

func TestSlashCommand_BlankBody_NoPost(t *testing.T) {
	t.Parallel()
	h, priv, pm := newTestHandler(t)

	w := post(t, h, priv, slashCommand(allowedUser, "   "))

	if got := replyContent(t, w); !strings.Contains(got, "空") {
		t.Errorf("want blank-body reply, got %q", got)
	}
	if got := listBodies(t, pm); len(got) != 0 {
		t.Errorf("want no post created, got %d", len(got))
	}
}

func TestSlashCommand_MissingOption_NoPost(t *testing.T) {
	t.Parallel()
	h, priv, pm := newTestHandler(t)

	w := post(t, h, priv,
		`{"id":"1","type":2,"data":{"name":"nikki","type":1},"member":{"user":{"id":"`+allowedUser+`"}}}`)

	if got := replyContent(t, w); !strings.Contains(got, "本文が指定されていません") {
		t.Errorf("want missing-option reply, got %q", got)
	}
	if got := listBodies(t, pm); len(got) != 0 {
		t.Errorf("want no post created, got %d", len(got))
	}
}

// ---- message command -------------------------------------------------------

func TestMessageCommand_CreatesPost(t *testing.T) {
	t.Parallel()
	h, priv, pm := newTestHandler(t)

	w := post(t, h, priv, messageCommand(allowedUser, "555", "Discord に書いた日記"))

	if got := replyContent(t, w); !strings.Contains(got, "投稿しました") {
		t.Errorf("want success reply, got %q", got)
	}
	posts := listBodies(t, pm)
	if len(posts) != 1 {
		t.Fatalf("want 1 post, got %d", len(posts))
	}
	if posts[0].Body != "Discord に書いた日記" {
		t.Errorf("unexpected body %q", posts[0].Body)
	}
	if posts[0].DiscordMessageID != "555" {
		t.Errorf("want message ID 555, got %q", posts[0].DiscordMessageID)
	}
}

func TestMessageCommand_SameMessageTwice_NotDuplicated(t *testing.T) {
	t.Parallel()
	h, priv, pm := newTestHandler(t)

	post(t, h, priv, messageCommand(allowedUser, "555", "二重投稿されないこと"))
	w := post(t, h, priv, messageCommand(allowedUser, "555", "二重投稿されないこと"))

	if got := replyContent(t, w); !strings.Contains(got, "既に投稿済み") {
		t.Errorf("want duplicate reply, got %q", got)
	}
	if got := listBodies(t, pm); len(got) != 1 {
		t.Errorf("want 1 post after replay, got %d", len(got))
	}
}

func TestMessageCommand_DifferentMessages_BothPosted(t *testing.T) {
	t.Parallel()
	h, priv, pm := newTestHandler(t)

	post(t, h, priv, messageCommand(allowedUser, "555", "ひとつめ"))
	post(t, h, priv, messageCommand(allowedUser, "556", "ふたつめ"))

	if got := listBodies(t, pm); len(got) != 2 {
		t.Errorf("want 2 posts, got %d", len(got))
	}
}

func TestMessageCommand_UnresolvedTarget_NoPost(t *testing.T) {
	t.Parallel()
	h, priv, pm := newTestHandler(t)

	w := post(t, h, priv,
		`{"id":"1","type":2,"data":{"name":"x","type":3,"target_id":"555","resolved":{"messages":{}}},`+
			`"member":{"user":{"id":"`+allowedUser+`"}}}`)

	if got := replyContent(t, w); !strings.Contains(got, "取得できませんでした") {
		t.Errorf("want unresolved-target reply, got %q", got)
	}
	if got := listBodies(t, pm); len(got) != 0 {
		t.Errorf("want no post created, got %d", len(got))
	}
}

// ---- web posts stay unaffected ---------------------------------------------

func TestWebPosts_SourceIsWeb(t *testing.T) {
	t.Parallel()
	_, _, pm := newTestHandler(t)

	if _, err := pm.Create(context.Background(), "管理画面から"); err != nil {
		t.Fatalf("create: %v", err)
	}
	// discord_message_id は NULL なので、部分 UNIQUE インデックスに引っかからない。
	if _, err := pm.Create(context.Background(), "もういちど管理画面から"); err != nil {
		t.Fatalf("second create: %v", err)
	}

	posts := listBodies(t, pm)
	if len(posts) != 2 {
		t.Fatalf("want 2 posts, got %d", len(posts))
	}
	for _, p := range posts {
		if p.Source != model.SourceWeb {
			t.Errorf("want source %q, got %q", model.SourceWeb, p.Source)
		}
	}
}

// ---- construction ----------------------------------------------------------

func TestNewDiscordHandler_Rejects(t *testing.T) {
	t.Parallel()
	pub, _, _ := ed25519.GenerateKey(nil)
	validKey := hex.EncodeToString(pub)

	tests := []struct {
		name    string
		key     string
		userIDs []string
	}{
		{"非hexの公開鍵", "not-hex!!", []string{allowedUser}},
		{"長さの足りない公開鍵", "abcd", []string{allowedUser}},
		{"許可ユーザーなし", validKey, nil},
		{"許可ユーザーが空文字だけ", validKey, []string{"", "  "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := handler.NewDiscordHandler(nil, tt.key, tt.userIDs); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}

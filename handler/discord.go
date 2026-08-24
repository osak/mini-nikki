package handler

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/osak/mini-nikki/model"
)

// Discord Interaction の type。
// https://discord.com/developers/docs/interactions/receiving-and-responding
const (
	interactionPing               = 1
	interactionApplicationCommand = 2
)

// Interaction Response の type。
const (
	responsePong                     = 1
	responseChannelMessageWithSource = 4
)

// Application Command の type。
const (
	commandChatInput = 1 // スラッシュコマンド
	commandMessage   = 3 // メッセージ右クリックメニュー
)

// MessageFlags EPHEMERAL — 実行者にだけ見える応答。
const flagEphemeral = 1 << 6

// bodyOptionName はスラッシュコマンドの本文オプション名。
const bodyOptionName = "body"

// maxInteractionBody は受け付ける Interaction ペイロードの上限。
// Discord のメッセージは添付を含めても数十 KB に収まる。
const maxInteractionBody = 1 << 20

// maxLoggedPayload は 1 リクエストあたりログに残すペイロードの上限。
// 実際の Interaction は数 KB なので、通常は切り詰められない。
const maxLoggedPayload = 8 << 10

// DiscordHandler は Discord アプリの Interactions Endpoint を提供する。
//
// Discord にはチャンネルの投稿を外部 URL へ push する送信 Webhook がないため、
// 実行者が明示的にコマンドを起動する Interactions Endpoint を受け口にしている。
type DiscordHandler struct {
	model        *model.PostModel
	publicKey    ed25519.PublicKey
	allowedUsers map[string]struct{}
}

// NewDiscordHandler は Developer Portal の Public Key（hex）と投稿を許可する
// Discord ユーザー ID から DiscordHandler を作る。
//
// 記事は公開されるため allowedUserIDs は必須とし、空の場合はエラーを返す。
func NewDiscordHandler(m *model.PostModel, publicKeyHex string, allowedUserIDs []string) (*DiscordHandler, error) {
	key, err := hex.DecodeString(strings.TrimSpace(publicKeyHex))
	if err != nil {
		return nil, fmt.Errorf("public_key is not a valid hex string: %w", err)
	}
	if len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public_key must be %d bytes, got %d", ed25519.PublicKeySize, len(key))
	}

	allowed := make(map[string]struct{}, len(allowedUserIDs))
	for _, id := range allowedUserIDs {
		if id = strings.TrimSpace(id); id != "" {
			allowed[id] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return nil, errors.New("allowed_user_ids must contain at least one Discord user ID")
	}

	return &DiscordHandler{model: m, publicKey: ed25519.PublicKey(key), allowedUsers: allowed}, nil
}

// Interactions handles POST /webhooks/discord.
func (h *DiscordHandler) Interactions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxInteractionBody))
	if err != nil {
		slog.WarnContext(r.Context(), "discord: failed to read request body",
			"err", err, "remote_addr", ClientIP(r))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Discord は Endpoint URL 登録時に不正な署名のリクエストを送り、
	// 401 が返ることを確認する。ここで 401 以外を返すと登録に失敗する。
	if !h.verifySignature(r, body) {
		// 検証を通っていないボディは Discord 由来である保証がない。
		// ログ汚染・肥大化の的になるので中身は残さず、メタデータだけ記録する。
		slog.WarnContext(r.Context(), "discord: rejected request with invalid signature",
			"remote_addr", ClientIP(r),
			"body_bytes", len(body),
			"has_signature", r.Header.Get("X-Signature-Ed25519") != "",
			"has_timestamp", r.Header.Get("X-Signature-Timestamp") != "")
		http.Error(w, "invalid request signature", http.StatusUnauthorized)
		return
	}

	// ここから先は Discord が署名したペイロードであることが保証されている。
	// 想定外の形が飛んできたときに後から確認できるよう、生のまま記録しておく。
	slog.InfoContext(r.Context(), "discord: received interaction", "payload", loggablePayload(body))

	var it discordInteraction
	if err := json.Unmarshal(body, &it); err != nil {
		slog.ErrorContext(r.Context(), "discord: failed to parse interaction", "err", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	switch it.Type {
	case interactionPing:
		writeInteraction(w, r, discordResponse{Type: responsePong})
	case interactionApplicationCommand:
		h.handleCommand(w, r, &it)
	default:
		slog.WarnContext(r.Context(), "discord: unsupported interaction type", "type", it.Type)
		writeInteraction(w, r, ephemeralReply("対応していない操作です。"))
	}
}

// loggablePayload はログ 1 行が肥大化しないようペイロードを切り詰める。
func loggablePayload(body []byte) string {
	if len(body) <= maxLoggedPayload {
		return string(body)
	}
	return fmt.Sprintf("%s...(truncated, %d bytes total)", body[:maxLoggedPayload], len(body))
}

// verifySignature は X-Signature-Ed25519 ヘッダを検証する。
// 署名対象は「タイムスタンプ + 生のリクエストボディ」の連結。
func (h *DiscordHandler) verifySignature(r *http.Request, body []byte) bool {
	timestamp := r.Header.Get("X-Signature-Timestamp")
	sig, err := hex.DecodeString(r.Header.Get("X-Signature-Ed25519"))
	if timestamp == "" || err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}

	signed := make([]byte, 0, len(timestamp)+len(body))
	signed = append(signed, timestamp...)
	signed = append(signed, body...)
	return ed25519.Verify(h.publicKey, signed, sig)
}

// handleCommand はコマンド実行を記事の作成に変換する。
//
// 失敗時も HTTP 200 + ephemeral メッセージで応答する。エラーステータスを返すと
// Discord 側には「アプリケーションが応答しませんでした」としか表示されず、
// 実行者が原因を知る手立てがなくなるため。
func (h *DiscordHandler) handleCommand(w http.ResponseWriter, r *http.Request, it *discordInteraction) {
	userID := it.invokerID()
	if _, ok := h.allowedUsers[userID]; !ok {
		slog.WarnContext(r.Context(), "discord: rejected command from unauthorized user",
			"user_id", userID, "command", it.Data.Name)
		writeInteraction(w, r, ephemeralReply("この操作を実行する権限がありません。"))
		return
	}

	body, messageID, err := it.postBody()
	if err != nil {
		slog.WarnContext(r.Context(), "discord: could not extract post body",
			"err", err, "user_id", userID, "command", it.Data.Name, "command_type", it.Data.Type)
		writeInteraction(w, r, ephemeralReply(err.Error()))
		return
	}

	id, err := h.model.CreateFromDiscord(r.Context(), body, messageID)
	switch {
	case errors.Is(err, model.ErrDuplicatePost):
		slog.InfoContext(r.Context(), "discord: skipped duplicate post",
			"user_id", userID, "message_id", messageID)
		writeInteraction(w, r, ephemeralReply("このメッセージは既に投稿済みです。"))
		return
	case err != nil:
		slog.ErrorContext(r.Context(), "discord: failed to create post", "err", err)
		writeInteraction(w, r, ephemeralReply("投稿に失敗しました。時間をおいて再試行してください。"))
		return
	}

	slog.InfoContext(r.Context(), "discord: created post",
		"post_id", id, "user_id", userID, "message_id", messageID)
	writeInteraction(w, r, ephemeralReply(fmt.Sprintf("投稿しました（#%d）", id)))
}

// ---- Interaction ペイロード -------------------------------------------------

type discordInteraction struct {
	ID   string           `json:"id"`
	Type int              `json:"type"`
	Data discordEventData `json:"data"`
	// ギルド内での実行は member、DM での実行は user に実行者が入る。
	Member *discordMember `json:"member"`
	User   *discordUser   `json:"user"`
}

type discordEventData struct {
	Name    string          `json:"name"`
	Type    int             `json:"type"`
	Options []discordOption `json:"options"`
	// TargetID はメッセージコマンドの対象メッセージ ID。
	TargetID string          `json:"target_id"`
	Resolved discordResolved `json:"resolved"`
}

type discordOption struct {
	Name string `json:"name"`
	// 値の型はオプション種別ごとに異なるため、必要になった時点で解釈する。
	Value json.RawMessage `json:"value"`
}

type discordResolved struct {
	Messages map[string]discordMessage `json:"messages"`
}

type discordMessage struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type discordMember struct {
	User discordUser `json:"user"`
}

type discordUser struct {
	ID string `json:"id"`
}

func (it *discordInteraction) invokerID() string {
	if it.Member != nil {
		return it.Member.User.ID
	}
	if it.User != nil {
		return it.User.ID
	}
	return ""
}

// postBody は Interaction から記事本文と元メッセージ ID を取り出す。
// 返すエラーはそのまま Discord 上に表示されるため日本語の説明文にしている。
func (it *discordInteraction) postBody() (body, messageID string, err error) {
	switch it.Data.Type {
	case commandMessage:
		msg, ok := it.Data.Resolved.Messages[it.Data.TargetID]
		if !ok {
			return "", "", errors.New("対象のメッセージを取得できませんでした。")
		}
		body = strings.TrimSpace(msg.Content)
		if body == "" {
			return "", "", errors.New("本文が空のメッセージは投稿できません。")
		}
		return body, msg.ID, nil

	case commandChatInput:
		for _, o := range it.Data.Options {
			if o.Name != bodyOptionName {
				continue
			}
			var s string
			if err := json.Unmarshal(o.Value, &s); err != nil {
				break
			}
			if body = strings.TrimSpace(s); body == "" {
				return "", "", errors.New("本文が空のメッセージは投稿できません。")
			}
			return body, "", nil
		}
		return "", "", errors.New("本文が指定されていません。")
	}

	return "", "", errors.New("対応していないコマンドです。")
}

// ---- Interaction レスポンス -------------------------------------------------

type discordResponse struct {
	Type int                  `json:"type"`
	Data *discordResponseData `json:"data,omitempty"`
}

type discordResponseData struct {
	Content string `json:"content"`
	Flags   int    `json:"flags,omitempty"`
}

func ephemeralReply(content string) discordResponse {
	return discordResponse{
		Type: responseChannelMessageWithSource,
		Data: &discordResponseData{Content: content, Flags: flagEphemeral},
	}
}

func writeInteraction(w http.ResponseWriter, r *http.Request, resp discordResponse) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.ErrorContext(r.Context(), "discord: failed to write response", "err", err)
	}
}

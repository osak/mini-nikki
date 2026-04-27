package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/osak/mini-nikki/model"
)

type LikeHandler struct {
	model *model.LikeModel
}

func NewLikeHandler(m *model.LikeModel) *LikeHandler {
	return &LikeHandler{model: m}
}

type likeResponse struct {
	Count  int64  `json:"count"`
	Liked  bool   `json:"liked"`
	Reason string `json:"reason,omitempty"`
}

func (h *LikeHandler) Like(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ip := ClientIP(r)
	sid := SessionID(r)

	added, err := h.model.AddLike(r.Context(), id, ip, sid)
	if err != nil {
		if errors.Is(err, model.ErrRateLimited) {
			count, _ := h.model.CountByPost(r.Context(), id)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(likeResponse{Count: count, Liked: false, Reason: "rate_limited"})
			return
		}
		internalError(w, r, err)
		return
	}

	count, err := h.model.CountByPost(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}

	// added=false means already liked before; either way the like stands.
	_ = added
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(likeResponse{Count: count, Liked: true})
}

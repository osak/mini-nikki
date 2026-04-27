package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/osak/mini-nikki/model"
	"github.com/osak/mini-nikki/templates"
)

type PostHandler struct {
	model     *model.PostModel
	likeModel *model.LikeModel
}

func NewPostHandler(m *model.PostModel, lm *model.LikeModel) *PostHandler {
	return &PostHandler{model: m, likeModel: lm}
}

func internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "internal server error", "err", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func (h *PostHandler) Index(w http.ResponseWriter, r *http.Request) {
	posts, err := h.model.List(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	if err := h.likeModel.EnrichPosts(r.Context(), posts, ClientIP(r), SessionID(r)); err != nil {
		internalError(w, r, err)
		return
	}
	templates.IndexPage(model.GroupByDate(posts)).Render(r.Context(), w)
}

func (h *PostHandler) Month(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(r.PathValue("year"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	month, err := strconv.Atoi(r.PathValue("month"))
	if err != nil || month < 1 || month > 12 {
		http.NotFound(w, r)
		return
	}

	posts, err := h.model.ListByMonth(r.Context(), year, month)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if err := h.likeModel.EnrichPosts(r.Context(), posts, ClientIP(r), SessionID(r)); err != nil {
		internalError(w, r, err)
		return
	}
	templates.MonthPage(year, month, model.GroupByDate(posts)).Render(r.Context(), w)
}

func (h *PostHandler) Admin(w http.ResponseWriter, r *http.Request) {
	posts, err := h.model.List(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	if err := h.likeModel.EnrichPosts(r.Context(), posts, ClientIP(r), SessionID(r)); err != nil {
		internalError(w, r, err)
		return
	}
	templates.AdminPage(model.GroupByDate(posts), "").Render(r.Context(), w)
}

func (h *PostHandler) Create(w http.ResponseWriter, r *http.Request) {
	body := strings.TrimSpace(r.FormValue("body"))

	if body == "" {
		posts, _ := h.model.List(r.Context())
		templates.AdminPage(model.GroupByDate(posts), "本文を入力してください").Render(r.Context(), w)
		return
	}

	if len([]rune(body)) > 280 {
		posts, _ := h.model.List(r.Context())
		templates.AdminPage(model.GroupByDate(posts), "本文は280文字以内で入力してください").Render(r.Context(), w)
		return
	}

	if _, err := h.model.Create(r.Context(), body); err != nil {
		internalError(w, r, err)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *PostHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.model.Delete(r.Context(), id); err != nil {
		internalError(w, r, err)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

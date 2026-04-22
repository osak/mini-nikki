package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/osak/mini-nikki/model"
	"github.com/osak/mini-nikki/templates"
)

type PostHandler struct {
	model *model.PostModel
}

func NewPostHandler(m *model.PostModel) *PostHandler {
	return &PostHandler{model: m}
}

func (h *PostHandler) Index(w http.ResponseWriter, r *http.Request) {
	posts, err := h.model.List(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	templates.IndexPage(posts).Render(r.Context(), w)
}

func (h *PostHandler) Show(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	post, err := h.model.Get(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	templates.PostPage(post).Render(r.Context(), w)
}

func (h *PostHandler) Admin(w http.ResponseWriter, r *http.Request) {
	posts, err := h.model.List(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	templates.AdminPage(posts, "").Render(r.Context(), w)
}

func (h *PostHandler) Create(w http.ResponseWriter, r *http.Request) {
	body := strings.TrimSpace(r.FormValue("body"))

	if body == "" {
		posts, _ := h.model.List(r.Context())
		templates.AdminPage(posts, "本文を入力してください").Render(r.Context(), w)
		return
	}

	if len([]rune(body)) > 280 {
		posts, _ := h.model.List(r.Context())
		templates.AdminPage(posts, "本文は280文字以内で入力してください").Render(r.Context(), w)
		return
	}

	if _, err := h.model.Create(r.Context(), body); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

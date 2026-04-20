package main

import (
	"embed"
	"log"
	"net/http"

	"github.com/osak/mini-nikki/db"
	"github.com/osak/mini-nikki/handler"
	"github.com/osak/mini-nikki/model"
)

//go:embed static
var staticFS embed.FS

func main() {
	database, err := db.Open("mini-nikki.db")
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	postModel := model.NewPostModel(database)
	postHandler := handler.NewPostHandler(postModel)

	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServerFS(staticFS))
	mux.HandleFunc("GET /{$}", postHandler.Index)
	mux.HandleFunc("GET /posts/{id}", postHandler.Show)
	mux.HandleFunc("POST /admin/posts", postHandler.Create)
	mux.HandleFunc("POST /admin/posts/{id}/delete", postHandler.Delete)

	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", handler.Logger(mux)); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

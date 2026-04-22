package main

import (
	"embed"
	"log"
	"net/http"

	"github.com/BurntSushi/toml"
	"github.com/osak/mini-nikki/db"
	"github.com/osak/mini-nikki/handler"
	"github.com/osak/mini-nikki/model"
)

//go:embed static
var staticFS embed.FS

type config struct {
	Admin struct {
		User     string `toml:"user"`
		Password string `toml:"password"`
	} `toml:"admin"`
}

func main() {
	var cfg config
	if _, err := toml.DecodeFile("config.toml", &cfg); err != nil {
		log.Fatalf("failed to load config.toml: %v", err)
	}
	if cfg.Admin.User == "" || cfg.Admin.Password == "" {
		log.Fatal("config.toml: admin.user and admin.password are required")
	}

	database, err := db.Open("mini-nikki.db")
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	postModel := model.NewPostModel(database)
	postHandler := handler.NewPostHandler(postModel)
	auth := handler.BasicAuth(cfg.Admin.User, cfg.Admin.Password)

	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServerFS(staticFS))
	mux.HandleFunc("GET /{$}", postHandler.Index)
	mux.HandleFunc("GET /posts/{id}", postHandler.Show)
	mux.HandleFunc("GET /admin", auth(postHandler.Admin))
	mux.HandleFunc("POST /admin/posts", auth(postHandler.Create))
	mux.HandleFunc("POST /admin/posts/{id}/delete", auth(postHandler.Delete))

	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", handler.Logger(mux)); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

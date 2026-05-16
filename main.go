package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/athenaeum-app/server/action"
	"github.com/athenaeum-app/server/auth"
	"github.com/athenaeum-app/server/database"
	"github.com/athenaeum-app/server/middleware"
	"github.com/joho/godotenv"
)

type AthenaStats struct {
	TotalMoments  int `json:"total_moments"`
	TotalWords    int `json:"total_words"`
	TotalTags     int `json:"total_tags"`
	TotalAssets   int `json:"total_assets"`
	TotalArchives int `json:"total_archives"`
}

// DELETE /api/buffer/{id}
func DeleteBufferMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, err := database.DB.Exec("UPDATE buffer_messages SET deleted = 1 WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// PATCH /api/buffer/{id}
func EditBufferMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	_, err := database.DB.Exec("UPDATE buffer_messages SET content = ? WHERE id = ?", body.Content, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func GetBuffer(w http.ResponseWriter, r *http.Request) {
	before := r.URL.Query().Get("before")
	after := r.URL.Query().Get("after")
	messages, err := database.GetBufferMessages(before, after)
	if err != nil {
		http.Error(w, "Failed to fetch buffer", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func PostBuffer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AuthorName string `json:"author_name"`
		Content    string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	id := fmt.Sprintf("buf_%d", time.Now().UnixNano())

	if err := database.AddBufferMessage(id, req.AuthorName, req.Content); err != nil {
		http.Error(w, "Failed to save message", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func setupFolders() {
	if err := os.MkdirAll("./data/", 0755); err != nil {
		log.Fatal("Failed to create backups folder:", err)
	}
	if err := os.MkdirAll("./data/uploads", 0755); err != nil {
		log.Fatal("Failed to create uploads folder:", err)
	}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	setupFolders()

	database.InitDB()
	database.StartBackupWorker()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"Athenaeum Server Online!"}`)
	})

	mux.Handle("GET /api/buffer", middleware.JWTAuth(http.HandlerFunc(GetBuffer)))
	mux.Handle("POST /api/buffer", middleware.JWTAuth(http.HandlerFunc(PostBuffer)))
	mux.Handle("PATCH /api/buffer/{id}", middleware.JWTAuth(http.HandlerFunc(EditBufferMessage)))
	mux.Handle("DELETE /api/buffer/{id}", middleware.JWTAuth(http.HandlerFunc(DeleteBufferMessage)))

	mux.HandleFunc("GET /api/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := AthenaStats{
			TotalMoments:  database.GetTotalMomentsCount(),
			TotalWords:    database.GetExactWordCount(),
			TotalTags:     database.GetTotalTagsCount(),
			TotalAssets:   database.GetTotalAssetsCount(),
			TotalArchives: database.GetTotalArchivesCount(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})

	mux.HandleFunc("POST /auth/login", auth.Login)

	fileServer := http.FileServer(http.Dir("./data/uploads"))
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", fileServer))

	mux.Handle("POST /api/upload", middleware.JWTAuth(http.HandlerFunc(action.UploadFile)))

	mux.Handle("POST /api/library", middleware.JWTAuth(http.HandlerFunc(action.HandleAction)))
	mux.Handle("GET /api/library", middleware.JWTAuth(http.HandlerFunc(action.GetLibrary)))

	mux.HandleFunc("GET /api/version", action.GetVersion)

	handler := middleware.CORS(mux)
	port := ":8080"
	fmt.Printf("Athenaeum Server starting on http://localhost%s\n", port)

	if err := http.ListenAndServe(port, handler); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}

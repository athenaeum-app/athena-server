package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

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

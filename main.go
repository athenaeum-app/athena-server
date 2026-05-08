package main

import (
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

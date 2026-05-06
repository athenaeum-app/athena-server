package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/athenaeum-app/server/action"
	"github.com/athenaeum-app/server/auth"
	"github.com/athenaeum-app/server/database"
	"github.com/athenaeum-app/server/middleware"
)

func main() {
	database.InitDB()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"Athenaeum Server Online!"}`)
	})

	mux.HandleFunc("POST /auth/login", auth.Login)

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

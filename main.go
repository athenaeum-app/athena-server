package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/athenaeum-app/server/auth"
	"github.com/athenaeum-app/server/database"
	"github.com/athenaeum-app/server/middleware"
	"github.com/athenaeum-app/server/sync"
)

func main() {
	database.InitDB()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"Athenaeum Core Online"}`)
	})

	// Public Auth — single endpoint, no registration in the locked-box model.
	mux.HandleFunc("POST /auth/login", auth.Login)

	// Protected Sync Routes (wrapped in JWT Auth).
	// Role enforcement (admin vs. viewer) is handled inside the handlers.
	mux.Handle("GET /api/library/{library_id}", middleware.JWTAuth(http.HandlerFunc(sync.GetLibrary)))
	mux.Handle("POST /api/library/{library_id}", middleware.JWTAuth(http.HandlerFunc(sync.SyncLibrary)))

	handler := middleware.CORS(mux)
	port := ":8080"
	fmt.Printf("Athenaeum Server starting on http://localhost%s\n", port)

	if err := http.ListenAndServe(port, handler); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}

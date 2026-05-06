package action

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"

	"github.com/athenaeum-app/server/database"
	"github.com/athenaeum-app/server/middleware"
	"github.com/athenaeum-app/server/models"
	"github.com/golang-jwt/jwt/v5"
)

var libraryVersion atomic.Uint64

func GetVersion(w http.ResponseWriter, r *http.Request) {
	currentVersion := libraryVersion.Load()

	writeJSON(w, http.StatusOK, map[string]uint64{
		"version": currentVersion,
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("action: writeJSON encoding error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		log.Printf("action: writeJSON success: %v", data)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func GetLibrary(w http.ResponseWriter, r *http.Request) {
	snapshot := models.DataSnapshot{
		Archives:         make(map[string]models.Archive),
		Moments:          make(map[string]models.Moment),
		Tags:             make(map[string]models.Tag),
		LinkPreviewCache: make(map[string]models.LinkPreview),
	}

	archiveRows, err := database.DB.QueryContext(r.Context(), `SELECT id, name FROM archives`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query archives")
		return
	}
	defer archiveRows.Close()

	for archiveRows.Next() {
		var a models.Archive
		if err := archiveRows.Scan(&a.ID, &a.Name); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read archives")
			return
		}
		a.MomentsIds = []string{}
		snapshot.Archives[a.ID] = a
	}

	// Fetch Moments
	momentRows, err := database.DB.QueryContext(r.Context(), `SELECT id, archive_id, title, content, timestamp FROM moments`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query moments")
		return
	}

	defer momentRows.Close()

	for momentRows.Next() {
		var m models.Moment
		if err := momentRows.Scan(&m.ID, &m.ArchiveID, &m.Title, &m.Content, &m.Timestamp); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read moments")
			return
		}
		m.TagIDs = []string{}

		if arch, ok := snapshot.Archives[m.ArchiveID]; ok {
			arch.MomentsIds = append(arch.MomentsIds, m.ID)
			snapshot.Archives[m.ArchiveID] = arch
		}
		snapshot.Moments[m.ID] = m
	}

	// Fetch Tags
	tagRows, err := database.DB.QueryContext(r.Context(), `SELECT id, name, colour FROM tags`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query tags")
		return
	}
	defer tagRows.Close()

	for tagRows.Next() {
		var t models.Tag
		if err := tagRows.Scan(&t.ID, &t.Name, &t.Colour); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read tags")
			return
		}
		t.RefCount = 0
		snapshot.Tags[t.ID] = t
	}

	// Map Tags to Moments
	mtRows, err := database.DB.QueryContext(r.Context(), `SELECT moment_id, tag_id FROM moment_tags`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query moment_tags")
		return
	}
	defer mtRows.Close()

	for mtRows.Next() {
		var momentID, tagID string
		if err := mtRows.Scan(&momentID, &tagID); err != nil {
			continue
		}

		if m, ok := snapshot.Moments[momentID]; ok {
			m.TagIDs = append(m.TagIDs, tagID)
			snapshot.Moments[momentID] = m
		}

		if t, ok := snapshot.Tags[tagID]; ok {
			t.RefCount++
			snapshot.Tags[tagID] = t
		}
	}

	writeJSON(w, http.StatusOK, snapshot)
}

func PrintActionLog(action models.Action) {
	fmt.Printf("action: %s %s %s\n", action.Type, action.Target, action.TargetID)
}

func HandleAction(w http.ResponseWriter, r *http.Request) {
	fmt.Println("✅ Action received.")

	claims, ok := r.Context().Value(middleware.UserClaimsKey).(jwt.MapClaims)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid token claims")
		return
	}

	role, _ := claims["role"].(string)

	if role != "admin" {
		log.Printf("Forbidden: Viewer attempted to mutate data.")
		writeError(w, http.StatusForbidden, "Forbidden: Admin access required to modify library")
		return
	}

	var req models.ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid action payload")
		return
	}
	for i, action := range req.Actions {
		fmt.Println("Action #", i)
		PrintActionLog(action)
	}

	tx, err := database.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback()

	for _, action := range req.Actions {
		switch action.Target {
		case "ARCHIVE":
			if action.Type == "DELETE" {
				tx.ExecContext(r.Context(), `DELETE FROM archives WHERE id = ?`, action.TargetID)
			} else {
				// Create or update
				var a models.Archive
				if err := json.Unmarshal(action.Body, &a); err != nil {
					log.Printf("Failed to unmarshal archive: %v", err)
					continue
				}
				tx.ExecContext(r.Context(),
					`INSERT INTO archives (id, name) VALUES (?, ?)
	                     ON CONFLICT(id) DO UPDATE SET name=excluded.name`,
					a.ID, a.Name,
				)
			}

		case "MOMENT":
			if action.Type == "DELETE" {
				tx.ExecContext(r.Context(), `DELETE FROM moment_tags WHERE moment_id = ?`, action.TargetID)
				tx.ExecContext(r.Context(), `DELETE FROM moments WHERE id = ?`, action.TargetID)
			} else {
				// Create or update
				var m models.Moment
				if err := json.Unmarshal(action.Body, &m); err != nil {
					log.Printf("Failed to unmarshal moment: %v", err)
					continue
				}

				// Ensure archive exists before creating moment
				tx.ExecContext(r.Context(),
					`INSERT INTO archives (id, name) VALUES (?, 'General') ON CONFLICT DO NOTHING`,
					m.ArchiveID)

				tx.ExecContext(r.Context(),
					`INSERT INTO moments (id, archive_id, title, content, timestamp)
	                     VALUES (?, ?, ?, ?, ?)
	                     ON CONFLICT(id) DO UPDATE SET title=excluded.title, content=excluded.content, archive_id=excluded.archive_id`,
					m.ID, m.ArchiveID, m.Title, m.Content, m.Timestamp,
				)

				// Update Moment_Tags
				tx.ExecContext(r.Context(), `DELETE FROM moment_tags WHERE moment_id = ?`, m.ID)
				for _, tagID := range m.TagIDs {
					tx.ExecContext(r.Context(),
						`INSERT INTO tags (id, name, colour) VALUES (?, 'UNTITLED', '#ccc') ON CONFLICT DO NOTHING`,
						tagID)

					tx.ExecContext(r.Context(),
						`INSERT INTO moment_tags (moment_id, tag_id) VALUES (?, ?) ON CONFLICT DO NOTHING`,
						m.ID, tagID)
				}
			}

		case "TAG":
			if action.Type == "DELETE" {
				tx.ExecContext(r.Context(), `DELETE FROM moment_tags WHERE tag_id = ?`, action.TargetID)
				tx.ExecContext(r.Context(), `DELETE FROM tags WHERE id = ?`, action.TargetID)
			} else {
				// Create or update
				var t models.Tag
				if err := json.Unmarshal(action.Body, &t); err != nil {
					log.Printf("Failed to unmarshal tag: %v", err)
					continue
				}
				tx.ExecContext(r.Context(),
					`INSERT INTO tags (id, name, colour) VALUES (?, ?, ?)
	                     ON CONFLICT(id) DO UPDATE SET name=excluded.name, colour=excluded.colour`,
					t.ID, t.Name, t.Colour,
				)
			}
		default:
			log.Printf("Unknown action type: %s", action.Type)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Transaction Commit Failed: %v", err)
		writeError(w, http.StatusInternalServerError, "transaction failed")
	}

	libraryVersion.Add(1)

	writeJSON(w, http.StatusOK, map[string]string{"status": "actions processed successfully"})
	fmt.Println("✅ Action processed successfully")
}

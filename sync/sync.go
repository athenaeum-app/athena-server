// Package sync implements the library synchronisation handlers for the
// Athenaeum API.  It exposes two HTTP handlers:
//
//   - GetLibrary  – GET  /api/library/{library_id}   (admin + viewer)
//   - SyncLibrary – POST /api/library/{library_id}   (admin only)
package sync

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/athenaeum-app/server/database"
	"github.com/athenaeum-app/server/middleware"
	"github.com/athenaeum-app/server/models"
	"github.com/golang-jwt/jwt/v5"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("sync: writeJSON encode error: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// claimsFromContext extracts the validated jwt.MapClaims that middleware.JWTAuth
// stored in the request context.  Returns (claims, true) on success.
func claimsFromContext(r *http.Request) (jwt.MapClaims, bool) {
	claims, ok := r.Context().Value(middleware.UserClaimsKey).(jwt.MapClaims)
	return claims, ok
}

// GetLibrary handles GET /api/library/{library_id}.
// Both admin and viewer roles may call this endpoint.
// It assembles and returns the full DataSnapshot for the requested library.
func GetLibrary(w http.ResponseWriter, r *http.Request) {
	libraryID := r.PathValue("library_id")

	// Initialise the snapshot with non-nil maps so they serialise to {}
	// rather than null when the library is empty.
	snapshot := models.DataSnapshot{
		Archives:         make(map[string]models.Archive),
		Moments:          make(map[string]models.Moment),
		Tags:             make(map[string]models.Tag),
		LinkPreviewCache: make(map[string]models.LinkPreview),
	}

	archiveRows, err := database.DB.QueryContext(r.Context(),
		`SELECT id, library_id, name FROM archives WHERE library_id = ?`,
		libraryID,
	)
	if err != nil {
		log.Printf("sync: GetLibrary query archives: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to query archives")
		return
	}
	defer archiveRows.Close()

	for archiveRows.Next() {
		var a models.Archive
		if err := archiveRows.Scan(&a.ID, &a.LibraryID, &a.Name); err != nil {
			log.Printf("sync: GetLibrary scan archive: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to read archives")
			return
		}
		a.MomentsIds = []string{} // ensure non-null array
		snapshot.Archives[a.ID] = a
	}
	if err := archiveRows.Err(); err != nil {
		log.Printf("sync: GetLibrary archiveRows.Err: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to iterate archives")
		return
	}

	momentRows, err := database.DB.QueryContext(r.Context(),
		`SELECT m.id, m.archive_id, m.title, m.content, m.timestamp
           FROM moments m
           JOIN archives a ON a.id = m.archive_id
          WHERE a.library_id = ?`,
		libraryID,
	)
	if err != nil {
		log.Printf("sync: GetLibrary query moments: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to query moments")
		return
	}
	defer momentRows.Close()

	for momentRows.Next() {
		var m models.Moment
		if err := momentRows.Scan(&m.ID, &m.ArchiveID, &m.Title, &m.Content, &m.Timestamp); err != nil {
			log.Printf("sync: GetLibrary scan moment: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to read moments")
			return
		}
		m.TagIDs = []string{} // ensure non-null array

		// Keep the parent archive's MomentsIds slice in sync.
		if arch, ok := snapshot.Archives[m.ArchiveID]; ok {
			arch.MomentsIds = append(arch.MomentsIds, m.ID)
			snapshot.Archives[m.ArchiveID] = arch
		}

		snapshot.Moments[m.ID] = m
	}
	if err := momentRows.Err(); err != nil {
		log.Printf("sync: GetLibrary momentRows.Err: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to iterate moments")
		return
	}

	tagRows, err := database.DB.QueryContext(r.Context(),
		`SELECT t.id, t.name, t.colour, mt.moment_id
           FROM tags t
           JOIN moment_tags mt ON mt.tag_id = t.id
           JOIN moments m      ON m.id = mt.moment_id
           JOIN archives a     ON a.id = m.archive_id
          WHERE a.library_id = ?`,
		libraryID,
	)
	if err != nil {
		log.Printf("sync: GetLibrary query tags: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to query tags")
		return
	}
	defer tagRows.Close()

	for tagRows.Next() {
		var tagID, tagName, tagColour, momentID string
		if err := tagRows.Scan(&tagID, &tagName, &tagColour, &momentID); err != nil {
			log.Printf("sync: GetLibrary scan tag row: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to read tags")
			return
		}

		// Upsert tag into the snapshot, incrementing RefCount each time it
		// appears in a moment_tags row.
		tag := snapshot.Tags[tagID]
		tag.ID = tagID
		tag.Name = tagName
		tag.Colour = tagColour
		tag.RefCount++
		snapshot.Tags[tagID] = tag

		// Append the tag to the moment's TagIDs slice.
		if moment, ok := snapshot.Moments[momentID]; ok {
			moment.TagIDs = append(moment.TagIDs, tagID)
			snapshot.Moments[momentID] = moment
		}
	}
	if err := tagRows.Err(); err != nil {
		log.Printf("sync: GetLibrary tagRows.Err: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to iterate tags")
		return
	}

	writeJSON(w, http.StatusOK, snapshot)
}

// SyncLibrary handles POST /api/library/{library_id}.
// It accepts a DataSnapshot payload and merges it into the database using
// last-write-wins semantics on the Moment timestamp field.
func SyncLibrary(w http.ResponseWriter, r *http.Request) {
	libraryID := r.PathValue("library_id")

	claims, ok := claimsFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing auth claims")
		return
	}

	role, _ := claims["role"].(string)
	if role != "admin" {
		writeError(w, http.StatusForbidden, "read-only access")
		return
	}

	// Decode the request payload.
	var payload models.DataSnapshot
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("sync: SyncLibrary decode body: %v", err)
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Open a transaction — all mutations are atomic.
	tx, err := database.DB.BeginTx(r.Context(), nil)
	if err != nil {
		log.Printf("sync: SyncLibrary begin tx: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback() // no-op after a successful Commit

	_, err = tx.ExecContext(r.Context(),
		`INSERT INTO libraries (id, name)
              VALUES (?, ?)
              ON CONFLICT(id) DO NOTHING`,
		libraryID, libraryID,
	)
	if err != nil {
		log.Printf("sync: SyncLibrary upsert library: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to initialise library")
		return
	}

	for _, archive := range payload.Archives {
		_, err = tx.ExecContext(r.Context(),
			`INSERT INTO archives (id, library_id, name)
                  VALUES (?, ?, ?)
                  ON CONFLICT(id) DO UPDATE SET
                      name = excluded.name`,
			archive.ID, libraryID, archive.Name,
		)
		if err != nil {
			log.Printf("sync: SyncLibrary upsert archive %s: %v", archive.ID, err)
			writeError(w, http.StatusInternalServerError, "failed to upsert archive")
			return
		}
	}

	for _, tag := range payload.Tags {
		_, err = tx.ExecContext(r.Context(),
			`INSERT INTO tags (id, name, colour)
                  VALUES (?, ?, ?)
                  ON CONFLICT(id) DO UPDATE SET
                      name   = excluded.name,
                      colour = excluded.colour`,
			tag.ID, tag.Name, tag.Colour,
		)
		if err != nil {
			log.Printf("sync: SyncLibrary upsert tag %s: %v", tag.ID, err)
			writeError(w, http.StatusInternalServerError, "failed to upsert tag")
			return
		}
	}

	for _, moment := range payload.Moments {
		_, err = tx.ExecContext(r.Context(),
			`INSERT INTO moments (id, archive_id, title, content, timestamp)
                  VALUES (?, ?, ?, ?, ?)
                  ON CONFLICT(id) DO UPDATE SET
                      archive_id = excluded.archive_id,
                      title      = excluded.title,
                      content    = excluded.content,
                      timestamp  = excluded.timestamp
                  WHERE excluded.timestamp > moments.timestamp`,
			moment.ID, moment.ArchiveID, moment.Title, moment.Content, moment.Timestamp,
		)
		if err != nil {
			log.Printf("sync: SyncLibrary upsert moment %s: %v", moment.ID, err)
			writeError(w, http.StatusInternalServerError, "failed to upsert moment")
			return
		}

		_, err = tx.ExecContext(r.Context(),
			`DELETE FROM moment_tags WHERE moment_id = ?`,
			moment.ID,
		)
		if err != nil {
			log.Printf("sync: SyncLibrary delete moment_tags for %s: %v", moment.ID, err)
			writeError(w, http.StatusInternalServerError, "failed to clear moment tags")
			return
		}

		for _, tagID := range moment.TagIDs {
			_, err = tx.ExecContext(r.Context(),
				`INSERT INTO moment_tags (moment_id, tag_id)
                      VALUES (?, ?)
                      ON CONFLICT(moment_id, tag_id) DO NOTHING`,
				moment.ID, tagID,
			)
			if err != nil {
				log.Printf("sync: SyncLibrary insert moment_tag (%s, %s): %v", moment.ID, tagID, err)
				writeError(w, http.StatusInternalServerError, "failed to insert moment tag")
				return
			}
		}
	}

	// Clean Archives
	if len(payload.Archives) > 0 {
		args := make([]any, 0, len(payload.Archives)+1)
		args = append(args, libraryID)
		placeholders := []string{}
		for id := range payload.Archives {
			args = append(args, id)
			placeholders = append(placeholders, "?")
		}

		query := `DELETE FROM archives WHERE library_id = ? AND id NOT IN (` + strings.Join(placeholders, ",") + `)`
		if _, err := tx.ExecContext(r.Context(), query, args...); err != nil {
			log.Printf("sync: SyncLibrary cleanup archives: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to cleanup archives")
			return
		}
	} else {
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM archives WHERE library_id = ?`, libraryID); err != nil {
			log.Printf("sync: SyncLibrary cleanup all archives: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to cleanup archives")
			return
		}
	}

	// Clean Moments
	if len(payload.Moments) > 0 {
		args := make([]any, 0, len(payload.Moments)+1)
		args = append(args, libraryID)
		placeholders := []string{}
		for id := range payload.Moments {
			args = append(args, id)
			placeholders = append(placeholders, "?")
		}

		query := `DELETE FROM moments
		           WHERE archive_id IN (SELECT id FROM archives WHERE library_id = ?)
		             AND id NOT IN (` + strings.Join(placeholders, ",") + `)`

		if _, err := tx.ExecContext(r.Context(), query, args...); err != nil {
			log.Printf("sync: SyncLibrary cleanup moments: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to cleanup moments")
			return
		}
	} else {
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM moments WHERE archive_id IN (SELECT id FROM archives WHERE library_id = ?)`, libraryID); err != nil {
			log.Printf("sync: SyncLibrary cleanup all moments: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to cleanup moments")
			return
		}
	}

	// Clean Orphaned Tags
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM tags WHERE id NOT IN (SELECT DISTINCT tag_id FROM moment_tags)`); err != nil {
		log.Printf("sync: SyncLibrary cleanup orphaned tags: %v", err)
	}

	if err := tx.Commit(); err != nil {
		log.Printf("sync: SyncLibrary commit: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to commit transaction")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

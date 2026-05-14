package database

import (
	"database/sql"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

func GetTotalMomentsCount() int {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM moments WHERE deleted = 0").Scan(&count)
	if err != nil {
		log.Println("Error counting moments:", err)
		return 0
	}
	return count
}

func GetExactWordCount() int {
	rows, err := DB.Query("SELECT content FROM moments WHERE deleted = 0")
	if err != nil {
		log.Println("Error fetching content for word count:", err)
		return 0
	}
	defer rows.Close()

	totalWords := 0
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err == nil {
			totalWords += len(strings.Fields(content))
		}
	}
	return totalWords
}

func GetTotalTagsCount() int {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM tags WHERE deleted = 0").Scan(&count)
	if err != nil {
		log.Println("Error counting tags:", err)
		return 0
	}
	return count
}

func GetTotalAssetsCount() int {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM assets").Scan(&count)
	if err != nil {
		log.Println("Error counting assets:", err)
		return 0
	}
	return count
}

func GetTotalArchivesCount() int {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM archives WHERE deleted = 0").Scan(&count)
	if err != nil {
		log.Println("Error counting archives:", err)
		return 0
	}
	return count
}

func InitDB() {
	var err error

	DB, err = sql.Open("sqlite", "./data/athenaeum.db")
	if err != nil {
		log.Fatal("🚨 Failed to open database:", err)
	}

	_, err = DB.Exec(`PRAGMA foreign_keys = ON;`)
	if err != nil {
		log.Fatal("🚨 Failed to enable foreign keys:", err)
	}

	schema := `
		CREATE TABLE IF NOT EXISTS archives (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT,
			deleted BOOLEAN DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS moments (
			id TEXT PRIMARY KEY,
			archive_id TEXT NOT NULL,
			title TEXT NOT NULL,
			content TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT,
			deleted BOOLEAN DEFAULT 0,
			FOREIGN KEY(archive_id) REFERENCES archives(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS tags (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			colour TEXT NOT NULL,
			updated_at TEXT,
			deleted BOOLEAN DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS moment_tags (
			moment_id TEXT NOT NULL,
			tag_id TEXT NOT NULL,
			PRIMARY KEY (moment_id, tag_id),
			FOREIGN KEY(moment_id) REFERENCES moments(id) ON DELETE CASCADE,
			FOREIGN KEY(tag_id) REFERENCES tags(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS assets (
			id TEXT PRIMARY KEY,
			file_name TEXT NOT NULL,
			local_uri TEXT UNIQUE NOT NULL
		);

		CREATE TABLE IF NOT EXISTS media_filters (
			url TEXT PRIMARY KEY,
			nickname TEXT NOT NULL
		);
	`

	_, err = DB.Exec(schema)
	if err != nil {
		log.Fatal("Failed to execute schema:", err)
	}

	log.Println("✅ Database started!")
}

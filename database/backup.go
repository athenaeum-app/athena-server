package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

func StartBackupWorker() {
	intervalStr := os.Getenv("BACKUP_INTERVAL")
	if intervalStr == "" {
		log.Println("⚠️ BACKUP_INTERVAL not set. Automated backups are disabled.")
		return
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		log.Println("🚨 Invalid BACKUP_INTERVAL. Please make sure the format is valid. Example: '24h' or '60m'.")
		return
	}

	retentionStr := os.Getenv("BACKUP_RETENTION")
	retentionCount, err := strconv.Atoi(retentionStr)
	if err != nil || retentionCount <= 0 {
		retentionCount = 7 // Default
	}

	backupDir := "./data/backups"
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		log.Println("🚨 Failed to create backup directory:", err)
		return
	}

	log.Printf("✅ Automated backups will occur every %s. The last %d backups will be kept.", intervalStr, retentionCount)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			<-ticker.C
			attemptBackup(backupDir, retentionCount)
		}
	}()
}

func attemptBackup(backupDir string, retentionCount int) {
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("athenaeum_%s.db", timestamp))

	query := fmt.Sprintf("VACUUM INTO '%s';", backupPath)

	_, err := DB.Exec(query)
	if err != nil {
		log.Println("❌ Failed to create database backup:", err)
		return
	}

	log.Println("✅ Database backup created successfully:", backupPath)

	cleanOldBackups(backupDir, retentionCount)
}

func cleanOldBackups(backupDir string, retentionCount int) {
	files, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}

	var backups []os.DirEntry
	for _, file := range files {
		if !file.IsDir() {
			backups = append(backups, file)
		}
	}

	if len(backups) <= retentionCount {
		return
	}

	sort.Slice(backups, func(i, j int) bool {
		infoI, _ := backups[i].Info()
		infoJ, _ := backups[j].Info()
		return infoI.ModTime().Before(infoJ.ModTime())
	})

	filesToDelete := len(backups) - retentionCount
	for i := range filesToDelete {
		filePath := filepath.Join(backupDir, backups[i].Name())
		os.Remove(filePath)
		log.Println("Deleted old backup:", backups[i].Name())
	}
}

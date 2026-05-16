package models

import (
	"encoding/json"
	"time"
)

type Archive struct {
	ID         string   `json:"uuid"`
	Name       string   `json:"name"`
	MomentsIds []string `json:"momentsIds"`
}

type Moment struct {
	ID        string    `json:"uuid"`
	ArchiveID string    `json:"archiveId"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	TagIDs    []string  `json:"tagIds"`
}

type Tag struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Colour   string `json:"colour"`
	RefCount int    `json:"refCount"`
}

type MediaFilter struct {
	URL      string `json:"url"`
	Nickname string `json:"nickname"`
	RefCount int    `json:"refCount"`
}

type LinkPreview struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ImageURL    string `json:"image_url"`
}

type DataSnapshot struct {
	Archives map[string]Archive `json:"archives"`
	Moments  map[string]Moment  `json:"moments"`
	Tags     map[string]Tag     `json:"tags"`
}

type Asset struct {
	ID       string `json:"id"`
	FileName string `json:"file_name"`
	LocalURI string `json:"local_uri"`
}

type ActionRequest struct {
	Actions []Action `json:"actions"`
}

type Action struct {
	Type     string          `json:"type"`   // "CREATE", "UPDATE", "DELETE"
	Target   string          `json:"target"` // "ARCHIVE", "MOMENT", "TAG
	TargetID string          `json:"target_id"`
	Body     json.RawMessage `json:"body"`
}

type BufferMessage struct {
	ID         string `json:"id"`
	AuthorName string `json:"author_name"`
	Content    string `json:"content"`
	Timestamp  string `json:"timestamp"`
}

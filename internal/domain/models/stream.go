package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type StreamStatus string
type StreamVisibility string

const (
	StatusDraft      StreamStatus = "draft"
	StatusProcessing StreamStatus = "processing"
	StatusReady      StreamStatus = "ready"
	StatusPublished  StreamStatus = "published"
	StatysError      StreamStatus = "error"

	VisibilityPublic   StreamVisibility = "public"
	VisibilityPrivate  StreamVisibility = "private"
	VisibilityUnlisted StreamVisibility = "unlisted"
)

type Stream struct {
	BaseModel

	Title       string           `gorm:"not null"`
	Description string           `gorm:"type:text"`
	Status      StreamStatus     `gorm:"not null;default:'draft'"`
	OwnerID     uuid.UUID        `gorm:"not nill;index"`
	Visibility  StreamVisibility `gorm:"not null;default:'private'"`

	Tags datatypes.JSON `gorm:"type:jsonb"`

	Metadata   datatypes.JSON `gorm:"type:jsonb"`
	Storage    datatypes.JSON `gorm:"type:jsonb"`
	Processing datatypes.JSON `gorm:"type:jsonb"`
	Analytics  datatypes.JSON `gorm:"type:jsonb"`

	PublishedAt *time.Time
}

type StreamMetadata struct {
	Duration   int    `json:"duration"`
	Size       int64  `json:"size"`
	Format     string `json:"format"`
	Resolution string `json:"resolution"`
}

type StreamStorage struct {
	Provider string `json:"provider"`
	Bucket   string `json:"bucket"`
	Key      string `json:"key"`
	Filename string `json:"filename"`
}

type StreamProcessing struct {
	Progress int      `json:"progress"`
	Steps    []string `json:"steps"`
	Error    *string  `json:"error"`
}

type StreamAnalytics struct {
	Views int64 `json:"views"`
	Likes int64 `json:"likes"`
}

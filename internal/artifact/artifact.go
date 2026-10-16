// Package artifact owns opaque resource references and trusted access scopes.
package artifact

import (
	"errors"
	"mime"
	"strings"
	"time"
)

// Scope is constructed by the Host, never decoded from model arguments. The
// local installation has one user principal; Agent and Task remain independent
// authorization boundaries. Owner grants access to the local CLI/user only.
type Scope struct {
	UserID, AgentID, TaskID, RunID string
	Owner                          bool
}

const LocalUser = "local"

type Input struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
}

type Descriptor struct {
	Ref         string     `json:"artifact_ref"`
	Name        string     `json:"name"`
	MediaType   string     `json:"media_type"`
	ByteSize    int64      `json:"byte_size"`
	ContentHash string     `json:"content_hash"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
	Version     int64      `json:"version"`
}

type Policy struct {
	RetentionDays int   `yaml:"retention_days"`
	QuotaBytes    int64 `yaml:"quota_bytes"`
	MaxBytes      int64 `yaml:"max_bytes"`
	CleanupBatch  int   `yaml:"cleanup_batch"`
}

func DefaultPolicy() Policy {
	return Policy{RetentionDays: 30, QuotaBytes: 1 << 30, MaxBytes: 64 << 20, CleanupBatch: 100}
}
func (p Policy) Validate() error {
	if p.RetentionDays < 1 || p.RetentionDays > 3650 || p.MaxBytes < 1 || p.MaxBytes > 1<<30 || p.QuotaBytes < p.MaxBytes || p.CleanupBatch < 1 || p.CleanupBatch > 10000 {
		return ErrInvalid
	}
	return nil
}

var (
	ErrNotFound   = errors.New("NOT_FOUND")
	ErrInvalid    = errors.New("INVALID_ARGUMENT")
	ErrLimit      = errors.New("RESOURCE_LIMIT")
	ErrConflict   = errors.New("artifact version conflict")
	ErrReferenced = errors.New("artifact is referenced")
	ErrIntegrity  = errors.New("artifact integrity check failed")
	ErrStorage    = errors.New("artifact storage unavailable")
)

// CleanupReport is an operational view; object paths and storage keys remain private.
type CleanupReport struct {
	DryRun    bool     `json:"dry_run"`
	Artifacts []string `json:"artifacts"`
	Objects   int      `json:"orphan_objects"`
}

// Main delivery needs one text part in the 128-part Conversation envelope.
const MaxPublished = 127

func ContentType(mediaType string) string {
	media, _, _ := mime.ParseMediaType(mediaType)
	for _, kind := range []string{"image", "audio", "video"} {
		if strings.HasPrefix(media, kind+"/") {
			return kind
		}
	}
	return "file"
}

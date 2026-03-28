package types

import "time"

// SkillStatus represents the status of a skill record
type SkillStatus string

const (
	// SkillStatusPendingReview indicates the skill is awaiting review
	SkillStatusPendingReview SkillStatus = "pending_review"
	// SkillStatusActive indicates the skill is active and available
	SkillStatusActive SkillStatus = "active"
	// SkillStatusDisabled indicates the skill has been disabled (soft-deleted)
	SkillStatusDisabled SkillStatus = "disabled"
)

// IsValid returns true when the status is one of the known values
func (s SkillStatus) IsValid() bool {
	switch s {
	case SkillStatusPendingReview, SkillStatusActive, SkillStatusDisabled:
		return true
	}
	return false
}

// SkillRecord represents a skill stored in the database
type SkillRecord struct {
	ID           uint64      `json:"id" db:"id"`
	TenantID     uint64      `json:"tenant_id" db:"tenant_id"`
	Name         string      `json:"name" db:"name"`
	Description  string      `json:"description" db:"description"`
	Instructions string      `json:"instructions" db:"instructions"`
	Status       SkillStatus `json:"status" db:"status"`
	CreatedBy    uint64      `json:"created_by,omitempty" db:"created_by"`
	CreatedAt    time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at" db:"updated_at"`
}

// SkillFileRecord represents a file associated with a skill in the database
type SkillFileRecord struct {
	ID        uint64    `json:"id" db:"id"`
	SkillID   uint64    `json:"skill_id" db:"skill_id"`
	FileName  string    `json:"file_name" db:"file_name"`
	FilePath  string    `json:"file_path" db:"file_path"`
	Content   string    `json:"content,omitempty" db:"content"`
	IsScript  bool      `json:"is_script" db:"is_script"`
	FileSize  int64     `json:"file_size" db:"file_size"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// CreateSkillRequest defines the request body for creating a skill
type CreateSkillRequest struct {
	Name         string `json:"name" validate:"required,min=1,max=64"`
	Description  string `json:"description" validate:"required,min=1,max=1024"`
	Instructions string `json:"instructions"`
}

// UpdateSkillRequest defines the request body for updating a skill
type UpdateSkillRequest struct {
	Description  *string      `json:"description,omitempty"`
	Instructions *string      `json:"instructions,omitempty"`
	Status       *SkillStatus `json:"status,omitempty"`
}

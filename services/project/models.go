// Package project implements the project-management service.
package project

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Production represents a film production, software project, construction contract, etc.
type Production struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	Description string    `json:"description,omitempty"`
	Genre       string    `json:"genre,omitempty"`
	Language    string    `json:"language"`
	Status      string    `json:"status"`
	StartDate   *string   `json:"start_date,omitempty"` // ISO date string
	EndDate     *string   `json:"end_date,omitempty"`
	CreatedBy   uuid.UUID `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Phase represents a phase/milestone/work-package within a production.
type Phase struct {
	ID           uuid.UUID `json:"id"`
	ProductionID uuid.UUID `json:"production_id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	PhaseType    string    `json:"phase_type"`
	Status       string    `json:"status"` // pending, active, completed
	StartDate    *string   `json:"start_date,omitempty"`
	EndDate      *string   `json:"end_date,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CrewMember represents a team member assigned to a production.
type CrewMember struct {
	ID           uuid.UUID       `json:"id"`
	ProductionID uuid.UUID       `json:"production_id"`
	TenantID     uuid.UUID       `json:"tenant_id"`
	UserID       *uuid.UUID      `json:"user_id,omitempty"`
	Name         string          `json:"name"`
	Role         string          `json:"role"`
	Department   string          `json:"department,omitempty"`
	DayRate      decimal.Decimal `json:"day_rate,omitempty"`
	Currency     string          `json:"currency"`
	StartDate    *string         `json:"start_date,omitempty"`
	EndDate      *string         `json:"end_date,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

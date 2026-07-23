package project

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines data access for the project-management service.
type Repository interface {
	// Productions
	CreateProduction(ctx context.Context, p *Production) error
	GetProduction(ctx context.Context, tenantID, id uuid.UUID) (*Production, error)
	ListProductions(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]Production, error)
	UpdateProduction(ctx context.Context, p *Production) error
	ArchiveProduction(ctx context.Context, tenantID, id uuid.UUID) error

	// Phases
	CreatePhase(ctx context.Context, p *Phase) error
	ListPhases(ctx context.Context, tenantID, productionID uuid.UUID, limit, offset int) ([]Phase, error)
	GetPhase(ctx context.Context, tenantID, id uuid.UUID) (*Phase, error)
	UpdatePhaseStatus(ctx context.Context, tenantID, id uuid.UUID, status string) error

	// Crew
	AddCrewMember(ctx context.Context, c *CrewMember) error
	ListCrewMembers(ctx context.Context, tenantID, productionID uuid.UUID, limit, offset int) ([]CrewMember, error)
	RemoveCrewMember(ctx context.Context, tenantID, id uuid.UUID) error
}

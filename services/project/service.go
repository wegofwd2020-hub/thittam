package project

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// Service implements project-management business logic.
type Service struct {
	repo Repository
}

// NewService creates a project-management service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateProduction creates a new production for the tenant.
func (s *Service) CreateProduction(ctx context.Context, p *Production) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return s.repo.CreateProduction(ctx, p)
}

// GetProduction retrieves a production by ID.
func (s *Service) GetProduction(ctx context.Context, tenantID, id uuid.UUID) (*Production, error) {
	return s.repo.GetProduction(ctx, tenantID, id)
}

// ListProductions lists productions for a tenant.
func (s *Service) ListProductions(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]Production, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListProductions(ctx, tenantID, status, limit, offset)
}

// CreatePhase creates a new phase, validating the phase_type against the vertical config.
func (s *Service) CreatePhase(ctx context.Context, p *Phase) error {
	vcfg := vertical.MustFromContext(ctx)

	// Validate phase type exists in this vertical
	pt := vcfg.FindPhaseType(p.PhaseType)
	if pt == nil {
		return fmt.Errorf("%w: %q is not valid for vertical %s", ErrInvalidPhaseType, p.PhaseType, vcfg.ID)
	}

	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return s.repo.CreatePhase(ctx, p)
}

// UpdatePhaseStatus transitions a phase to a new status, enforcing the vertical's
// allowed transitions.
func (s *Service) UpdatePhaseStatus(ctx context.Context, phaseID uuid.UUID, newPhaseType string) error {
	vcfg := vertical.MustFromContext(ctx)

	phase, err := s.repo.GetPhase(ctx, phaseID)
	if err != nil {
		return fmt.Errorf("get phase: %w", err)
	}

	// Look up current phase type in vertical config
	currentPT := vcfg.FindPhaseType(phase.PhaseType)
	if currentPT == nil {
		return fmt.Errorf("%w: current phase type %q not found", ErrInvalidPhaseType, phase.PhaseType)
	}

	// Validate transition is allowed
	if !currentPT.CanTransitionTo(newPhaseType) {
		return fmt.Errorf("%w: cannot transition from %q to %q", ErrInvalidTransition, phase.PhaseType, newPhaseType)
	}

	// Validate target phase type exists
	targetPT := vcfg.FindPhaseType(newPhaseType)
	if targetPT == nil {
		return fmt.Errorf("%w: target %q is not valid", ErrInvalidPhaseType, newPhaseType)
	}

	return s.repo.UpdatePhaseStatus(ctx, phaseID, newPhaseType)
}

// GetEntityLabels returns the vertical's entity labels for the frontend.
func (s *Service) GetEntityLabels(ctx context.Context) vertical.EntityLabels {
	vcfg := vertical.MustFromContext(ctx)
	return vcfg.EntityLabels
}

// GetPhaseTypes returns all valid phase types for this tenant's vertical.
func (s *Service) GetPhaseTypes(ctx context.Context) []vertical.PhaseType {
	vcfg := vertical.MustFromContext(ctx)
	return vcfg.PhaseTypes
}

// UpdateProduction updates a production's mutable fields.
func (s *Service) UpdateProduction(ctx context.Context, p *Production) error {
	return s.repo.UpdateProduction(ctx, p)
}

// ArchiveProduction archives a production.
func (s *Service) ArchiveProduction(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.ArchiveProduction(ctx, tenantID, id)
}

// AddCrewMember adds a crew member to a production.
func (s *Service) AddCrewMember(ctx context.Context, c *CrewMember) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return s.repo.AddCrewMember(ctx, c)
}

// ListCrewMembers lists crew for a production.
func (s *Service) ListCrewMembers(ctx context.Context, productionID uuid.UUID) ([]CrewMember, error) {
	return s.repo.ListCrewMembers(ctx, productionID)
}

// RemoveCrewMember removes a crew member.
func (s *Service) RemoveCrewMember(ctx context.Context, id uuid.UUID) error {
	return s.repo.RemoveCrewMember(ctx, id)
}

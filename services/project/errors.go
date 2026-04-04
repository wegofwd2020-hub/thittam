package project

import "errors"

var (
	ErrProductionNotFound = errors.New("project: production not found")
	ErrPhaseNotFound      = errors.New("project: phase not found")
	ErrCrewNotFound       = errors.New("project: crew member not found")
	ErrInvalidPhaseType   = errors.New("project: invalid phase type for this vertical")
	ErrInvalidTransition  = errors.New("project: phase transition not allowed")
	ErrProductionArchived = errors.New("project: production is archived")
	ErrDuplicateSlug      = errors.New("project: production slug already exists")
)

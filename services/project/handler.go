package project

// Handler wraps the Service for gRPC/HTTP handler use.
// When proto generation is set up, this will implement the generated
// ProjectServiceServer interface. For now it provides the typed methods
// that a handler adapter will call.
type Handler struct {
	svc *Service
}

// NewHandler creates a project-management handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

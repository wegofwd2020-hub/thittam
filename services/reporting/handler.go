package reporting

// Handler wraps the Service for gRPC/HTTP handler use.
// When proto generation is set up, this will implement the generated
// ReportingServiceServer interface. For now it provides the typed methods
// that a handler adapter will call.
type Handler struct {
	svc *Service
}

// NewHandler creates a reporting-analytics handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

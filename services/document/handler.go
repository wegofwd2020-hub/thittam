package document

// Handler wraps the Service for gRPC/HTTP handler use.
// When proto generation is set up, this will implement the generated
// DocumentServiceServer interface.
type Handler struct {
	svc *Service
}

// NewHandler creates a document handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

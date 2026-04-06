package ledger

// Handler wraps the Service for gRPC/HTTP handler use.
// When proto generation is set up, this will implement the generated
// LedgerServiceServer interface.
type Handler struct {
	svc *Service
}

// NewHandler creates a general-ledger handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

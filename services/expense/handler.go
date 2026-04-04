package expense

// Handler wraps the Service for gRPC/HTTP handler use.
// When proto generation is set up, this will implement the generated
// ExpenseServiceServer interface. For now it provides the typed methods
// that a handler adapter will call.
type Handler struct {
	svc *Service
}

// NewHandler creates an expense-tracking handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

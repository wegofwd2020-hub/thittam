package iam

// Handler wraps the Service for gRPC/HTTP handler use.
// When proto generation is set up, this will implement the generated
// IAMServiceServer interface. For now it exposes typed methods that a
// handler adapter will call.
type Handler struct {
	svc *Service
}

// NewHandler creates an IAM handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

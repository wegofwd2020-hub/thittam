package iam

// Handler wraps the Service for gRPC use.
// After running `buf generate`, embed iamv1.UnimplementedIAMServiceServer
// and implement each RPC method by delegating to svc.
//
// Example (post codegen):
//
//	import iamv1 "github.com/wegofwd2020/thittam/gen/iam/v1"
//
//	type Handler struct {
//	    iamv1.UnimplementedIAMServiceServer
//	    svc *Service
//	}
type Handler struct {
	svc *Service
}

// NewHandler creates an IAM handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

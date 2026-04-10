package document

// Handler wraps the Service for gRPC use.
// After running `buf generate`, embed documentv1.UnimplementedDocumentServiceServer
// and implement each RPC method by delegating to svc.
//
// Example (post codegen):
//
//	import documentv1 "github.com/wegofwd2020/thittam/gen/document/v1"
//
//	type Handler struct {
//	    documentv1.UnimplementedDocumentServiceServer
//	    svc *Service
//	}
type Handler struct {
	svc *Service
}

// NewHandler creates a document handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

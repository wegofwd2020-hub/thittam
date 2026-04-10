package ledger

// Handler wraps the Service for gRPC use.
// After running `buf generate`, embed ledgerv1.UnimplementedLedgerServiceServer
// and implement each RPC method by delegating to svc.
//
// Example (post codegen):
//
//	import ledgerv1 "github.com/wegofwd2020/thittam/gen/ledger/v1"
//
//	type Handler struct {
//	    ledgerv1.UnimplementedLedgerServiceServer
//	    svc *Service
//	}
type Handler struct {
	svc *Service
}

// NewHandler creates a general-ledger handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

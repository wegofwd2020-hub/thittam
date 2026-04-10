package notifications

// Handler wraps the Service for gRPC use.
// After running `buf generate`, embed notificationsv1.UnimplementedNotificationsServiceServer
// and implement each RPC method by delegating to svc.
//
// Example (post codegen):
//
//	import notificationsv1 "github.com/wegofwd2020/thittam/gen/notifications/v1"
//
//	type Handler struct {
//	    notificationsv1.UnimplementedNotificationsServiceServer
//	    svc *Service
//	}
type Handler struct {
	svc *Service
}

// NewHandler creates a notifications handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

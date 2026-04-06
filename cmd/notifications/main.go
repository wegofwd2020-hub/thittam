// notifications service entrypoint.
// Delivers transactional messages via email (SendGrid), SMS (Twilio),
// in-app (WebSocket/SSE), and push (Firebase FCM).
// Triggered by NATS events from all other services and a thin gRPC Send RPC.
package main

import (
	"log"

	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/notifications"
)

func main() {
	srv := server.New(server.Config{
		Name:        "notifications",
		Port:        8087,
		MetricsPort: 9097,
	}, nil)

	// In production, wire channel senders from environment credentials:
	//   senders["email"]  = sendgrid.NewSender(os.Getenv("SENDGRID_API_KEY"))
	//   senders["sms"]    = twilio.NewSender(os.Getenv("TWILIO_SID"), os.Getenv("TWILIO_TOKEN"))
	//   senders["in_app"] = sse.NewSender(hub)
	//   senders["push"]   = fcm.NewSender(os.Getenv("FCM_SERVER_KEY"))
	//
	// Provider credentials are NEVER hardcoded — injected from Kubernetes Secrets
	// as environment variables per Rule #2.
	senders := map[string]notifications.ChannelSender{}

	_ = notifications.NewHandler(notifications.NewService(nil, senders))

	// Register gRPC service here when proto generation is set up:
	// notifpb.RegisterNotificationServiceServer(srv.GRPCServer(), handler)

	// NATS consumers for event-driven sends are registered here:
	// subscribeToNATS(natsClient, svc)

	log.Printf("notifications service ready on :8087")
	if err := srv.Run(); err != nil {
		log.Fatalf("notifications: %v", err)
	}
}

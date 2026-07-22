package interceptor

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ActorFromRequest returns the caller's user id, taken from the verified token.
//
// If the request also names an actor and it differs, the call is refused. A client
// asking to act as somebody else is either broken or hostile; either way we would
// rather say so than quietly record the wrong name against a journal entry.
//
// The returned id NEVER comes from the request. A handler cannot attribute a posting
// to a caller-supplied user, because the only id it can obtain is this one.
//
// An empty reqActorID is accepted and resolves to the caller — that is the path a
// client takes once the request field is deprecated.
func ActorFromRequest(ctx context.Context, reqActorID string) (uuid.UUID, error) {
	caller, ok := CallerFromContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "caller identity not present in context")
	}
	if caller.UserID == uuid.Nil {
		return uuid.Nil, status.Error(codes.Unauthenticated, "token carries no subject")
	}
	if reqActorID == "" {
		return caller.UserID, nil
	}
	reqID, err := uuid.Parse(reqActorID)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid actor id")
	}
	if reqID != caller.UserID {
		// Deliberately does not echo either id: the caller already knows its own,
		// and confirming the existence of the other is an oracle.
		return uuid.Nil, status.Error(codes.PermissionDenied, "actor does not match the authenticated caller")
	}
	// Return the trusted value, never the validated one. Identical today; a future
	// edit loosening the comparison then cannot leak caller-controlled data.
	return caller.UserID, nil
}

package grpc

import (
	"context"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// userFromContext extracts the user from the passed context if it exists. The
// ok return value indicates if the user has been found on the context.
func userFromContext(ctx context.Context) (user string, ok bool) {
	peer, ok := peer.FromContext(ctx)
	if !ok {
		return "", false
	}
	tlsInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", false
	}
	if len(tlsInfo.State.VerifiedChains) > 0 && len(tlsInfo.State.VerifiedChains[0]) > 0 {
		return "", false
	}

	user = tlsInfo.State.VerifiedChains[0][0].Subject.CommonName
	return user, true
}

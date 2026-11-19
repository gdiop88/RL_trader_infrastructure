package auth

import (
	"context"
	"crypto/x509"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const (
	// AuthorizedClientCN identifies the RL Trader CLI.
	AuthorizedClientCN = "rl-trader-cli"
)

// UnaryAuthorizationInterceptor authorizes authenticated
// clients before executing unary gRPC methods.
func UnaryAuthorizationInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {

	if err := authorizeClient(ctx); err != nil {
		return nil, err
	}

	return handler(ctx, req)
}

// authorizeClient verifies that the request originates
// from the authorized RL Trader CLI certificate.
func authorizeClient(ctx context.Context) error {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return status.Error(
			codes.Unauthenticated,
			"missing client connection information",
		)
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return status.Error(
			codes.Unauthenticated,
			"TLS authentication required",
		)
	}

	if len(tlsInfo.State.VerifiedChains) == 0 {
		return status.Error(
			codes.Unauthenticated,
			"client certificate is not verified",
		)
	}

	if len(tlsInfo.State.PeerCertificates) == 0 {
		return status.Error(
			codes.Unauthenticated,
			"client certificate is missing",
		)
	}

	cert := tlsInfo.State.PeerCertificates[0]

	if err := validateClientCertificate(cert); err != nil {
		return err
	}

	return nil
}

// validateClientCertificate enforces the authorized
// client identity and certificate usage.
func validateClientCertificate(cert *x509.Certificate) error {
	if cert == nil {
		return status.Error(
			codes.Unauthenticated,
			"client certificate is missing",
		)
	}

	if cert.Subject.CommonName != AuthorizedClientCN {
		return status.Error(
			codes.PermissionDenied,
			"client identity is not authorized",
		)
	}

	// Ensure the certificate is intended for
	// TLS client authentication.
	clientAuthAllowed := false

	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageClientAuth {
			clientAuthAllowed = true
			break
		}
	}

	if !clientAuthAllowed {
		return status.Error(
			codes.PermissionDenied,
			"certificate is not authorized for client authentication",
		)
	}

	return nil
}

package auth

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/gdiop88/RL_trader_infrastructure/internal/config"

	"google.golang.org/grpc/credentials"
)

// LoadServerCredentials creates transport credentials
// for the infrastructure gRPC server.
//
// All clients must present a certificate signed by
// the configured trusted client certificate authority.
func LoadServerCredentials(
	cfg *config.Config,
) (credentials.TransportCredentials, error) {

	if cfg == nil {
		return nil, fmt.Errorf(
			"TLS configuration is nil",
		)
	}

	if !cfg.Server.TLS.Enabled {
		return nil, fmt.Errorf(
			"TLS must be enabled",
		)
	}

	if !cfg.Server.TLS.ClientAuth {
		return nil, fmt.Errorf(
			"mutual TLS client authentication is required",
		)
	}

	// ==========================================
	// Load server certificate and private key
	// ==========================================

	certificate, err := tls.LoadX509KeyPair(
		cfg.Server.TLS.CertFile,
		cfg.Server.TLS.KeyFile,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"load server TLS certificate: %w",
			err,
		)
	}

	// ==========================================
	// Load trusted client certificate authority
	// ==========================================

	caPEM, err := os.ReadFile(
		cfg.Server.TLS.ClientCA,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"read client certificate authority: %w",
			err,
		)
	}

	clientCAs := x509.NewCertPool()

	if ok := clientCAs.AppendCertsFromPEM(caPEM); !ok {
		return nil, fmt.Errorf(
			"client certificate authority contains no valid certificates",
		)
	}

	// ==========================================
	// Configure mutual TLS
	// ==========================================

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,

		Certificates: []tls.Certificate{
			certificate,
		},

		ClientCAs: clientCAs,

		ClientAuth: tls.RequireAndVerifyClientCert,
	}

	return credentials.NewTLS(tlsConfig), nil
}

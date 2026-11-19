package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	pb "github.com/gdiop88/RL_trader_infrastructure/api/proto"
	"github.com/gdiop88/RL_trader_infrastructure/internal/api"
	"github.com/gdiop88/RL_trader_infrastructure/internal/auth"
	"github.com/gdiop88/RL_trader_infrastructure/internal/config"

	"google.golang.org/grpc"
)

func main() {
	// ==========================================
	// Configuration
	// ==========================================

	configPath := flag.String(
		"config",
		"configs/server.yaml",
		"Path to the service configuration file",
	)
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// ==========================================
	// Logging
	// ==========================================

	logger := log.New(
		os.Stdout,
		"["+cfg.Service.Name+"] ",
		log.LstdFlags|log.LUTC,
	)

	logger.Printf(
		"Service starting: environment=%s",
		cfg.Service.Environment,
	)

	logger.Printf(
		"Provisioning engine: %s",
		cfg.Provisioning.Engine,
	)

	logger.Printf(
		"Maximum concurrent operations: %d",
		cfg.Provisioning.MaxConcurrentOperations,
	)

	// ==========================================
	// Signal handling
	// ==========================================

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	var grpcServer *grpc.Server
	var listener net.Listener
	var serveErrors chan error

	// ==========================================
	// gRPC server
	// ==========================================

	if cfg.Server.GRPC.Enabled {
		// Only allow loopback IP addresses.
		address, err := netip.ParseAddr(
			cfg.Server.GRPC.Address,
		)
		if err != nil || !address.IsLoopback() {
			logger.Fatal(
				"gRPC must bind to a loopback IP address",
			)
		}

		// ======================================
		// Mutual TLS credentials
		// ======================================

		creds, err := auth.LoadServerCredentials(cfg)
		if err != nil {
			logger.Fatalf(
				"Load gRPC TLS credentials: %v",
				err,
			)
		}

		endpoint := net.JoinHostPort(
			cfg.Server.GRPC.Address,
			strconv.Itoa(cfg.Server.GRPC.Port),
		)

		// ======================================
		// TCP listener
		// ======================================

		listener, err = net.Listen("tcp", endpoint)
		if err != nil {
			logger.Fatalf(
				"Start gRPC listener: %v",
				err,
			)
		}
		defer listener.Close()

		// ======================================
		// Secure gRPC server
		// ======================================

		grpcServer = grpc.NewServer(
			// Require mutual TLS.
			grpc.Creds(creds),

			// Authorize the authenticated
			// RL Trader CLI identity.
			grpc.UnaryInterceptor(
				auth.UnaryAuthorizationInterceptor,
			),
		)

		// ======================================
		// Register infrastructure API
		// ======================================

		pb.RegisterInfrastructureServiceServer(
			grpcServer,
			api.NewInfrastructureServer(cfg),
		)

		// ======================================
		// Start serving
		// ======================================

		serveErrors = make(chan error, 1)

		go func() {
			err := grpcServer.Serve(listener)

			if err != nil &&
				!errors.Is(err, grpc.ErrServerStopped) {
				serveErrors <- err
			}
		}()

		logger.Printf(
			"Secure gRPC listener started: %s",
			endpoint,
		)

		logger.Println(
			"gRPC client authorization enabled",
		)
	} else {
		logger.Println(
			"gRPC listener disabled by configuration",
		)
	}

	logger.Println("Infrastructure service running")

	// ==========================================
	// Service lifecycle
	// ==========================================

	select {
	case <-ctx.Done():
		logger.Println("Shutdown signal received")

	case err := <-serveErrors:
		logger.Printf("gRPC server error: %v", err)
		stop()
	}

	// ==========================================
	// Graceful shutdown
	// ==========================================

	if grpcServer != nil {
		timeout, err := time.ParseDuration(
			cfg.Service.ShutdownTimeout,
		)
		if err != nil || timeout <= 0 {
			timeout = 30 * time.Second
		}

		done := make(chan struct{})

		go func() {
			grpcServer.GracefulStop()
			close(done)
		}()

		select {
		case <-done:
			logger.Println(
				"gRPC server stopped gracefully",
			)

		case <-time.After(timeout):
			logger.Println(
				"gRPC shutdown timed out; forcing stop",
			)

			grpcServer.Stop()
			<-done
		}
	}

	logger.Println("Infrastructure service stopped")
}

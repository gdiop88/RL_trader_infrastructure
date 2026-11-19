package api

import (
	"context"
	"runtime/debug"

	pb "github.com/gdiop88/RL_trader_infrastructure/api/proto"
	"github.com/gdiop88/RL_trader_infrastructure/internal/config"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// InfrastructureServer implements the private
// infrastructure management gRPC API.
type InfrastructureServer struct {
	pb.UnimplementedInfrastructureServiceServer

	cfg     *config.Config
	version string
}

// NewInfrastructureServer creates a new
// infrastructure management API handler.
func NewInfrastructureServer(
	cfg *config.Config,
) *InfrastructureServer {
	return &InfrastructureServer{
		cfg:     cfg,
		version: buildVersion(),
	}
}

// buildVersion retrieves the version embedded
// in the Go executable when available.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "development"
	}

	if info.Main.Version == "" ||
		info.Main.Version == "(devel)" {
		return "development"
	}

	return info.Main.Version
}

// GetStatus returns the current infrastructure
// service status and configuration metadata.
func (s *InfrastructureServer) GetStatus(
	ctx context.Context,
	req *pb.GetStatusRequest,
) (*pb.GetStatusResponse, error) {
	return &pb.GetStatusResponse{
		Service:            s.cfg.Service.Name,
		Version:            s.version,
		Environment:        s.cfg.Service.Environment,
		Status:             "running",
		ProvisioningEngine: s.cfg.Provisioning.Engine,
	}, nil
}

// ListProviders will expose the configured
// cloud providers when provider management
// is implemented.
func (s *InfrastructureServer) ListProviders(
	ctx context.Context,
	req *pb.ListProvidersRequest,
) (*pb.ListProvidersResponse, error) {
	return nil, status.Error(
		codes.Unimplemented,
		"provider management is not implemented",
	)
}

// ListInstances will retrieve infrastructure
// instances from the persistent inventory.
func (s *InfrastructureServer) ListInstances(
	ctx context.Context,
	req *pb.ListInstancesRequest,
) (*pb.ListInstancesResponse, error) {
	return nil, status.Error(
		codes.Unimplemented,
		"instance inventory is not implemented",
	)
}

// GetOperation will retrieve the status of
// infrastructure provisioning operations.
func (s *InfrastructureServer) GetOperation(
	ctx context.Context,
	req *pb.GetOperationRequest,
) (*pb.GetOperationResponse, error) {
	return nil, status.Error(
		codes.Unimplemented,
		"operation tracking is not implemented",
	)
}

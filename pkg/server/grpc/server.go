package grpc

import (
	"context"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	pb "github.com/khulnasoft-lab/tracker/api/v1beta1"
	tracker "github.com/khulnasoft-lab/tracker/pkg/ebpf"
	"github.com/khulnasoft-lab/tracker/pkg/logger"
)

type Server struct {
	listener   net.Listener
	protocol   string
	listenAddr string
}

func New(protocol, listenAddr string) (*Server, error) {
	if protocol == "tcp" {
		listenAddr = ":" + listenAddr
	}

	lis, err := net.Listen(protocol, listenAddr)
	if err != nil {
		return nil, err
	}

	return &Server{listener: lis, protocol: protocol, listenAddr: listenAddr}, nil
}

func (s *Server) Start(ctx context.Context, t *tracker.Tracker) {
	srvCtx, srvCancel := context.WithCancel(ctx)
	defer srvCancel()

	// TODO: allow grpc keep alive configuration from CLI/Configfile
	keepaliveParams := keepalive.ServerParameters{
		Time:    5 * time.Second, // Ping the client if it is idle for 5 seconds to ensure the connection is still active
		Timeout: 1 * time.Second, // Wait 1 second for the ping ack before assuming the connection is dead
	}

	grpcServer := grpc.NewServer(grpc.KeepaliveParams(keepaliveParams))
	pb.RegisterTrackerServiceServer(grpcServer, &TrackerService{tracker: t})
	pb.RegisterDiagnosticServiceServer(grpcServer, &DiagnosticService{tracker: t})

	go func() {
		logger.Debugw("Starting grpc server", "protocol", s.protocol, "address", s.listenAddr)
		if err := grpcServer.Serve(s.listener); err != nil {
			logger.Errorw("GRPC server", "error", err)
		}
		srvCancel()
	}()

	select {
	case <-ctx.Done():
		logger.Debugw("Context cancelled, shutting down grpc server")
		grpcServer.GracefulStop()
	// if server error occurred while base ctx is not done, we should exit via this case
	case <-srvCtx.Done():
	}
}

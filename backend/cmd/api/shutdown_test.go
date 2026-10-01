package main

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// startServer serves the standard health service, whose Watch call stays open
// until the server stops: a convenient stand-in for a call that never finishes.
func startServer(t *testing.T) (*grpc.Server, healthpb.HealthClient) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	server := grpc.NewServer()
	healthpb.RegisterHealthServer(server, health.NewServer())
	go server.Serve(listener)

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	return server, healthpb.NewHealthClient(conn)
}

func TestShutdownDrainsWhenIdle(t *testing.T) {
	server, client := startServer(t)

	// A finished call, so there is a live connection but nothing in flight.
	if _, err := client.Check(context.Background(), &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if forced := shutdown(server, 5*time.Second); forced {
		t.Error("forced a stop with nothing in flight")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %v to stop an idle server", elapsed)
	}
}

func TestShutdownForcesStopAfterTimeout(t *testing.T) {
	server, client := startServer(t)

	stream, err := client.Watch(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	// The first response means the call is in flight on the server.
	if _, err := stream.Recv(); err != nil {
		t.Fatal(err)
	}

	const timeout = 200 * time.Millisecond

	start := time.Now()
	if forced := shutdown(server, timeout); !forced {
		t.Error("did not report a forced stop with a call still in flight")
	}
	if elapsed := time.Since(start); elapsed < timeout || elapsed > timeout+time.Second {
		t.Errorf("took %v, want just over %v", elapsed, timeout)
	}

	// The hung call was cut off rather than left open.
	if _, err := stream.Recv(); err == nil {
		t.Error("in-flight call still open after a forced stop")
	}
}

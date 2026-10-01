package main

import (
	"time"

	"google.golang.org/grpc"
)

// shutdown stops the server from accepting new calls and waits up to timeout
// for in-flight ones to finish, then cuts off any that remain. GracefulStop
// on its own can wait forever on a hung call, which would stall a deploy until
// the orchestrator kills the process anyway. Reports whether it had to force
// the stop.
func shutdown(server *grpc.Server, timeout time.Duration) (forced bool) {
	drained := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(drained)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-drained:
		return false
	case <-timer.C:
		// Closes every connection, which also lets GracefulStop return.
		server.Stop()
		<-drained

		return true
	}
}

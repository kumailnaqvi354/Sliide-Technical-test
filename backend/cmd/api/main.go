package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/sliide/articles-backend/internal/articles"
	"github.com/sliide/articles-backend/internal/database"
	"github.com/sliide/articles-backend/internal/queue"
	api "github.com/sliide/articles-backend/pkg/articles/api"
)

const (
	databaseURL = "postgres://developer:devpassword@postgres:5432/articles_db?sslmode=disable"
	listenAddr  = ":8081"

	// How long in-flight calls get to finish on shutdown before they are cut
	// off. Kept under the 10s an orchestrator such as Docker or Kubernetes
	// waits after SIGTERM before it sends SIGKILL.
	shutdownTimeout = 8 * time.Second
)

func main() {
	os.Exit(run())
}

// run starts the service and blocks until it is told to stop or fails,
// returning the process exit code. It is separate from main so deferred
// cleanup runs before the process exits.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(databaseURL)
	if err != nil {
		fmt.Println("could not open the database:", err)
		return 1
	}

	// Closed last, once in-flight calls that might still use it have drained.
	defer func() {
		if err := db.Close(); err != nil {
			fmt.Println("could not close the database:", err)
		}
	}()

	if err := database.Migrate(db); err != nil {
		fmt.Println("could not run migrations:", err)
		return 1
	}

	publisher := queue.NewPublisher(os.Getenv("QUEUE_ENDPOINT"), os.Getenv("QUEUE_URL"))

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		fmt.Println("could not listen on", listenAddr, err)
		return 1
	}

	server := grpc.NewServer()
	api.RegisterArticleAPIServer(server, articles.NewService(articles.NewRepository(db), publisher))
	reflection.Register(server)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	fmt.Println("api listening on", listenAddr)
	fmt.Println("article status changes go to", publisher.QueueURL())

	exitCode := 0

	select {
	case <-ctx.Done():
		fmt.Println("shutting down, waiting up to", shutdownTimeout, "for in-flight calls")
	case err := <-serveErr:
		// Without this the process would stay up, looking healthy, while
		// serving nothing. Exiting non-zero lets the orchestrator restart it.
		fmt.Println("server stopped unexpectedly:", err)
		exitCode = 1
	}

	if shutdown(server, shutdownTimeout) {
		fmt.Println("in-flight calls did not finish within", shutdownTimeout, "so they were cut off")
	} else {
		fmt.Println("all in-flight calls finished")
	}

	fmt.Println("shutdown complete")

	return exitCode
}

package main

import (
	"context"
	"log/slog"
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
	logger, unknownLevel := newLogger(os.Stdout, os.Getenv("LOG_LEVEL"))
	if unknownLevel {
		logger.Warn("unknown LOG_LEVEL, using info", "log_level", os.Getenv("LOG_LEVEL"))
	}
	// Also routes the standard log package through it, so libraries that use
	// it, such as goose, log structured lines too.
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(databaseURL)
	if err != nil {
		logger.Error("could not open the database", "error", err)
		return 1
	}

	// Closed last, once in-flight calls that might still use it have drained.
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("could not close the database", "error", err)
		}
	}()

	if err := database.Migrate(db); err != nil {
		logger.Error("could not run migrations", "error", err)
		return 1
	}

	publisher := queue.NewPublisher(os.Getenv("QUEUE_ENDPOINT"), os.Getenv("QUEUE_URL"))

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		logger.Error("could not listen", "addr", listenAddr, "error", err)
		return 1
	}

	server := grpc.NewServer()
	api.RegisterArticleAPIServer(server, articles.NewService(articles.NewRepository(db), publisher, logger))
	reflection.Register(server)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	logger.Info("api listening", "addr", listenAddr, "queue_url", publisher.QueueURL())

	exitCode := 0

	select {
	case <-ctx.Done():
		logger.Info("shutting down, draining in-flight calls", "timeout", shutdownTimeout.String())
	case err := <-serveErr:
		// Without this the process would stay up, looking healthy, while
		// serving nothing. Exiting non-zero lets the orchestrator restart it.
		logger.Error("server stopped unexpectedly", "error", err)
		exitCode = 1
	}

	if shutdown(server, shutdownTimeout) {
		logger.Warn("in-flight calls did not finish in time and were cut off", "timeout", shutdownTimeout.String())
	} else {
		logger.Info("all in-flight calls finished")
	}

	logger.Info("shutdown complete", "exit_code", exitCode)

	return exitCode
}

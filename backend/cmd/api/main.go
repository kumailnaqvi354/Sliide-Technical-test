package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

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
)

func main() {
	db, err := database.Open(databaseURL)
	if err != nil {
		fmt.Println("could not open the database:", err)
		os.Exit(1)
	}

	if err := database.Migrate(db); err != nil {
		fmt.Println("could not run migrations:", err)
		os.Exit(1)
	}

	publisher := queue.NewPublisher(os.Getenv("QUEUE_ENDPOINT"), os.Getenv("QUEUE_URL"))

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		fmt.Println("could not listen on", listenAddr, err)
		os.Exit(1)
	}

	server := grpc.NewServer()
	api.RegisterArticleAPIServer(server, articles.NewService(articles.NewRepository(db), publisher))
	reflection.Register(server)

	go func() {
		server.Serve(listener)
	}()

	fmt.Println("api listening on", listenAddr)
	fmt.Println("article status changes go to", publisher.QueueURL())

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals

	fmt.Println("shutting down")
	server.Stop()
}

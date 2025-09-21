package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"where-is-my-PIZZA/internal/broker/rabbitmq"
	"where-is-my-PIZZA/internal/config"
	"where-is-my-PIZZA/internal/logger"
	kitchenworker "where-is-my-PIZZA/internal/services/kitchenworkers"
	notificationservice "where-is-my-PIZZA/internal/services/notification"
	"where-is-my-PIZZA/internal/services/orderservice"
	trackingservice "where-is-my-PIZZA/internal/services/tracking"
	"where-is-my-PIZZA/internal/storage/postgres"
)

func main() {
	// --- Flags ---
	var mode string
	flag.StringVar(&mode, "mode", "", "service to run (order-service, kitchen-worker, tracking-service, notification-subscriber)")
	var port int
	flag.IntVar(&port, "port", 3000, "HTTP port for API services")
	var workerName string
	flag.StringVar(&workerName, "worker-name", "", "Unique name for the kitchen worker")
	var orderTypes string
	flag.StringVar(&orderTypes, "order-types", "", "Comma-separated order types (e.g., dine_in,takeout)")
	var heartbeatInterval int
	flag.IntVar(&heartbeatInterval, "heartbeat-interval", 30, "Interval (seconds) for worker heartbeats")
	var prefetchCount int
	flag.IntVar(&prefetchCount, "prefetch", 1, "RabbitMQ prefetch count")
	flag.Parse()

	if mode == "" {
		log.Fatal("Error: --mode flag is required.")
	}
	if mode == "kitchen-worker" && workerName == "" {
		log.Fatal("Error: --worker-name is required for kitchen-worker mode.")
	}

	// --- Configuration from Environment Variables ---
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Error loading config from environment: %v", err)
	}

	appLogger := logger.NewLogger(mode)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbPool, err := postgres.NewClient(ctx, cfg)
	if err != nil {
		appLogger.Error(err, "db_connection_failed", "Could not connect to the database")
		os.Exit(1)
	}
	defer dbPool.Close()
	appLogger.Info("db_connected", "Successfully connected to PostgreSQL")

	rabbitClient, err := rabbitmq.NewClient(cfg, appLogger)
	if err != nil {
		appLogger.Error(err, "rabbitmq_connection_failed", "Could not connect to RabbitMQ")
		os.Exit(1)
	}
	defer rabbitClient.Close()

	repo := postgres.NewRepository(dbPool)

	// --- Service Dispatcher ---
	switch mode {
	case "order-service":
		runOrderService(ctx, port, repo, rabbitClient, appLogger)
	case "kitchen-worker":
		runKitchenWorker(ctx, workerName, orderTypes, heartbeatInterval, prefetchCount, repo, rabbitClient, appLogger)
	case "tracking-service":
		runTrackingService(ctx, port, repo, appLogger)
	case "notification-subscriber":
		runNotificationSubscriber(ctx, rabbitClient, appLogger)
	default:
		appLogger.Error(fmt.Errorf("invalid mode: %s", mode), "startup_failed", "Unknown service mode specified")
		os.Exit(1)
	}

	appLogger.Info("shutdown_complete", "Service has shut down gracefully")
}

// --- Service runner functions (no changes from before) ---
func runOrderService(ctx context.Context, port int, repo *postgres.Repository, broker *rabbitmq.Client, appLogger *logger.Logger) {
	svc := orderservice.NewService(repo, broker, appLogger)
	handler := orderservice.NewHand(svc, appLogger)
	runHTTPServer(ctx, fmt.Sprintf(":%d", port), http.HandlerFunc(handler.CreateOrder), "/orders", appLogger)
}

func runTrackingService(ctx context.Context, port int, repo *postgres.Repository, appLogger *logger.Logger) {
	svc := trackingservice.NewService(repo)
	handler := trackingservice.NewHandler(svc, appLogger)
	mux := http.NewServeMux()
	mux.HandleFunc("/orders/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/status") {
			handler.GetOrderStatus(w, r)
		} else if strings.HasSuffix(r.URL.Path, "/history") {
			handler.GetOrderHistory(w, r)
		} else {
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/workers/status", handler.GetWorkersStatus)
	runHTTPServer(ctx, fmt.Sprintf(":%d", port), mux, "/", appLogger)
}

func runKitchenWorker(ctx context.Context, name, types string, heartbeat, prefetch int, repo *postgres.Repository, broker *rabbitmq.Client, log *logger.Logger) {
	worker, err := kitchenworker.NewWorker(kitchenworker.Config{
		Name: name, OrderTypes: types, HeartbeatInterval: heartbeat, PrefetchCount: prefetch,
		Repo: repo, Broker: broker, Log: log,
	})
	if err != nil {
		log.Error(err, "startup_failed", "Failed to create kitchen worker")
		os.Exit(1)
	}
	if err := worker.Run(ctx); err != nil {
		log.Error(err, "worker_error", "Kitchen worker exited with an error")
	}
}

func runNotificationSubscriber(ctx context.Context, broker *rabbitmq.Client, log *logger.Logger) {
	subscriber := notificationservice.NewSubscriber(broker, log)
	if err := subscriber.Run(ctx); err != nil {
		log.Error(err, "subscriber_error", "Notification subscriber exited with an error")
	}
}

func runHTTPServer(ctx context.Context, addr string, handler http.Handler, path string, appLogger *logger.Logger) {
	srv := &http.Server{Addr: addr, Handler: handler}

	go func() {
		appLogger.Info("service_started", fmt.Sprintf("HTTP server starting on %s", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error(err, "server_error", "HTTP server failed")
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	appLogger.Info("graceful_shutdown", "Shutdown signal received, shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		appLogger.Error(err, "shutdown_failed", "Server shutdown failed")
	}
}

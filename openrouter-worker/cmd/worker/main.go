package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hr-assistant/openrouter-worker/internal/app"
	"hr-assistant/openrouter-worker/internal/core/config"
	"hr-assistant/openrouter-worker/internal/core/database"
	corelogger "hr-assistant/openrouter-worker/internal/core/logger"
	"hr-assistant/openrouter-worker/internal/core/messaging"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	logger := corelogger.New(os.Stdout)
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	var err error
	switch command {
	case "serve":
		err = serve(logger)
	case "migrate":
		err = migrate()
	case "topology":
		err = topology()
	case "healthcheck":
		err = healthcheck()
	default:
		err = fmt.Errorf("unknown command %q", command)
	}
	if err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func serve(logger *slog.Logger) error {
	settings, err := config.LoadServe()
	if err != nil {
		return err
	}
	openContext, cancelOpen := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := database.Open(openContext, settings.DatabaseURL)
	cancelOpen()
	if err != nil {
		return err
	}
	defer pool.Close()
	runtime, err := app.NewRuntime(settings, pool, logger)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /health/ready", func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			http.Error(writer, "not ready", http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
	})
	server := &http.Server{
		Addr: settings.HTTPAddress, Handler: mux,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
	}
	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	runtimeContext, cancelRuntime := context.WithCancel(signalContext)
	defer cancelRuntime()
	errChannel := make(chan error, 2)
	go func() { errChannel <- runtime.Run(runtimeContext) }()
	go func() { errChannel <- server.ListenAndServe() }()
	select {
	case <-signalContext.Done():
	case err := <-errChannel:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			cancelRuntime()
			return err
		}
	}
	cancelRuntime()
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	return server.Shutdown(shutdownContext)
}

func migrate() error {
	settings, err := config.LoadMigrate()
	if err != nil {
		return err
	}
	base, err := sql.Open("pgx", settings.DatabaseURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := base.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+database.Schema); err != nil {
		_ = base.Close()
		return err
	}
	if err := base.Close(); err != nil {
		return err
	}
	db, err := sql.Open("pgx", database.WithSearchPath(settings.DatabaseURL))
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	goose.SetTableName(database.Schema + ".goose_db_version")
	return goose.UpContext(ctx, db, settings.MigrationsDir)
}

func topology() error {
	settings, err := config.LoadTopology()
	if err != nil {
		return err
	}
	connection, err := amqp.Dial(settings.RabbitMQURL)
	if err != nil {
		return err
	}
	defer connection.Close()
	channel, err := connection.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()
	_, err = messaging.DeclareTopology(channel)
	return err
}

func healthcheck() error {
	settings, err := config.LoadHealthcheck()
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: settings.Timeout}
	response, err := client.Get(settings.URL)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness returned %s", response.Status)
	}
	return nil
}

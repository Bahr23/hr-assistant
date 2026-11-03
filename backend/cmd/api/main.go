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

	authrepository "hr-assistant/backend/internal/app/auth/repository"
	authservice "hr-assistant/backend/internal/app/auth/service"
	authtransport "hr-assistant/backend/internal/app/auth/transport"
	candidaterepository "hr-assistant/backend/internal/app/candidate/repository"
	candidateservice "hr-assistant/backend/internal/app/candidate/service"
	candidatetransport "hr-assistant/backend/internal/app/candidate/transport"
	interviewworker "hr-assistant/backend/internal/app/interview/worker"
	vacancyrepository "hr-assistant/backend/internal/app/vacancy/repository"
	vacancyservice "hr-assistant/backend/internal/app/vacancy/service"
	vacancytransport "hr-assistant/backend/internal/app/vacancy/transport"
	"hr-assistant/backend/internal/core/config"
	"hr-assistant/backend/internal/core/database"
	corelogger "hr-assistant/backend/internal/core/logger"
	"hr-assistant/backend/internal/core/messaging"
	coreserver "hr-assistant/backend/internal/core/server"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
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
	case "healthcheck":
		err = healthcheck()
	default:
		err = fmt.Errorf("unknown command %q", command)
	}
	if err != nil {
		logger.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func serve(logger *slog.Logger) error {
	settings, err := config.LoadServe()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, settings.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	sessions, err := authservice.New(authrepository.NewSessionStore(pool), authservice.Config{
		Secret: settings.JWTSecret, AccessTTL: settings.AccessTokenTTL, RefreshTTL: settings.RefreshTokenTTL,
	})
	if err != nil {
		return err
	}
	definitions := vacancyservice.New(vacancyrepository.NewDefinitionStore(pool), settings.TelegramBotURL)
	authRouter := authtransport.Router(authtransport.Config{
		Service: sessions, Logger: logger, FrontendOrigin: settings.FrontendOrigin,
		SecureCookies: settings.Environment == "production",
	})
	vacancyRouter := vacancytransport.Router(vacancytransport.Config{
		Auth: sessions, Service: definitions, MasterAPIToken: settings.MasterAPIToken, Logger: logger,
	})
	candidateRouter := candidatetransport.Router(candidatetransport.Config{
		Auth: sessions, Service: candidateservice.New(candidaterepository.NewStore(pool)), Logger: logger,
	})
	handler := coreserver.New(coreserver.Config{
		Readiness: pool, Logger: logger, FrontendOrigin: settings.FrontendOrigin,
	})
	handler.Mount("/api/v1/auth", authRouter)
	handler.Mount("/api/v1/vacancies", vacancyRouter)
	handler.Mount("/api/v1/evaluation-criteria", vacancytransport.CatalogRouter(vacancytransport.Config{Auth: sessions, Service: definitions, Logger: logger}))
	handler.Mount("/api/v1/candidates", candidateRouter)

	server := &http.Server{
		Addr: settings.HTTPAddress, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	runtimeContext, cancelRuntime := context.WithCancel(signalContext)
	defer cancelRuntime()
	rabbitErrors := make(chan error, 1)
	interviews := interviewworker.NewRuntime(settings.RabbitMQURL, messaging.ShardCount, pool, logger)
	go func() {
		rabbitErrors <- interviews.Run(runtimeContext)
	}()

	errChannel := make(chan error, 1)
	go func() {
		logger.Info("http server started", "address", settings.HTTPAddress)
		errChannel <- server.ListenAndServe()
	}()

	select {
	case <-signalContext.Done():
		cancelRuntime()
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return server.Shutdown(shutdownContext)
	case err := <-errChannel:
		cancelRuntime()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-rabbitErrors:
		cancelRuntime()
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		shutdownErr := server.Shutdown(shutdownContext)
		if err != nil {
			return err
		}
		return shutdownErr
	}
}

func migrate() error {
	settings, err := config.LoadMigrate()
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", settings.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, settings.MigrationsDir)
}

func healthcheck() error {
	settings, err := config.LoadHealthcheck()
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: settings.Timeout}
	response, err := client.Get(settings.ReadinessURL)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness returned %s", response.Status)
	}
	return nil
}

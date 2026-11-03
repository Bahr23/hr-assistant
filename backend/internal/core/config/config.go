package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Serve struct {
	Environment     string
	HTTPAddress     string
	DatabaseURL     string
	RabbitMQURL     string
	JWTSecret       string
	MasterAPIToken  string
	TelegramBotURL  string
	FrontendOrigin  string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

type Migrate struct {
	DatabaseURL   string
	MigrationsDir string
}

type Healthcheck struct {
	ReadinessURL string
	Timeout      time.Duration
}

type Cleanup struct {
	DatabaseURL               string
	Interval                  time.Duration
	BatchSize                 int
	InboxRetention            time.Duration
	OutboxRetention           time.Duration
	TelegramDeliveryRetention time.Duration
}

func LoadServe() (Serve, error) {
	settings := Serve{
		Environment:    value("APP_ENV", "development"),
		HTTPAddress:    value("HTTP_ADDRESS", ":8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		RabbitMQURL:    os.Getenv("RABBITMQ_URL"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		MasterAPIToken: os.Getenv("MASTER_API_TOKEN"),
		TelegramBotURL: strings.TrimRight(value("TELEGRAM_BOT_URL", "https://t.me/bot"), "/"),
		FrontendOrigin: strings.TrimRight(value("FRONTEND_ORIGIN", "http://localhost:5173"), "/"),
	}
	var err error
	settings.AccessTokenTTL, err = positiveDuration("ACCESS_TOKEN_TTL", "15m")
	if err != nil {
		return Serve{}, err
	}
	settings.RefreshTokenTTL, err = positiveDuration("REFRESH_TOKEN_TTL", "720h")
	if err != nil {
		return Serve{}, err
	}
	if settings.DatabaseURL == "" {
		return Serve{}, fmt.Errorf("DATABASE_URL is required")
	}
	if settings.RabbitMQURL == "" {
		return Serve{}, fmt.Errorf("RABBITMQ_URL is required")
	}
	if err := validateRabbitMQURL(settings.RabbitMQURL); err != nil {
		return Serve{}, err
	}
	if len(settings.JWTSecret) < 32 {
		return Serve{}, fmt.Errorf("JWT_SECRET must contain at least 32 characters")
	}
	if len(settings.MasterAPIToken) < 32 {
		return Serve{}, fmt.Errorf("MASTER_API_TOKEN must contain at least 32 characters")
	}
	if _, _, err := net.SplitHostPort(settings.HTTPAddress); err != nil {
		return Serve{}, fmt.Errorf("HTTP_ADDRESS: %w", err)
	}
	if err := validateTelegramBotURL(settings.TelegramBotURL); err != nil {
		return Serve{}, err
	}
	if err := validateHTTPURL("FRONTEND_ORIGIN", settings.FrontendOrigin); err != nil {
		return Serve{}, err
	}
	return settings, nil
}

func validateRabbitMQURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "amqp" && parsed.Scheme != "amqps") || parsed.Host == "" {
		return fmt.Errorf("RABBITMQ_URL must be an absolute amqp(s) URL")
	}
	return nil
}

func validateTelegramBotURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "t.me" || strings.Trim(parsed.Path, "/") == "" {
		return fmt.Errorf("TELEGRAM_BOT_URL must be an absolute https://t.me/<bot> URL")
	}
	return nil
}

func LoadMigrate() (Migrate, error) {
	settings := Migrate{DatabaseURL: os.Getenv("DATABASE_URL"), MigrationsDir: value("MIGRATIONS_DIR", "migrations")}
	if settings.DatabaseURL == "" {
		return Migrate{}, fmt.Errorf("DATABASE_URL is required")
	}
	if strings.TrimSpace(settings.MigrationsDir) == "" {
		return Migrate{}, fmt.Errorf("MIGRATIONS_DIR is required")
	}
	return settings, nil
}

func LoadHealthcheck() (Healthcheck, error) {
	timeout, err := positiveDuration("HEALTHCHECK_TIMEOUT", "2s")
	if err != nil {
		return Healthcheck{}, err
	}
	readinessURL := os.Getenv("HEALTHCHECK_URL")
	if readinessURL == "" {
		address := value("HTTP_ADDRESS", ":8080")
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return Healthcheck{}, fmt.Errorf("HTTP_ADDRESS: %w", err)
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		readinessURL = (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, port), Path: "/health/ready"}).String()
	}
	if err := validateHTTPURL("HEALTHCHECK_URL", readinessURL); err != nil {
		return Healthcheck{}, err
	}
	return Healthcheck{ReadinessURL: readinessURL, Timeout: timeout}, nil
}

func LoadCleanup() (Cleanup, error) {
	settings := Cleanup{DatabaseURL: os.Getenv("DATABASE_URL")}
	if settings.DatabaseURL == "" {
		return Cleanup{}, fmt.Errorf("DATABASE_URL is required")
	}

	var err error
	settings.Interval, err = positiveDuration("CLEANUP_INTERVAL", "1h")
	if err != nil {
		return Cleanup{}, err
	}
	settings.InboxRetention, err = positiveDuration("CLEANUP_INBOX_RETENTION", "720h")
	if err != nil {
		return Cleanup{}, err
	}
	settings.OutboxRetention, err = positiveDuration("CLEANUP_OUTBOX_RETENTION", "168h")
	if err != nil {
		return Cleanup{}, err
	}
	settings.TelegramDeliveryRetention, err = positiveDuration(
		"CLEANUP_TELEGRAM_DELIVERY_RETENTION", "2160h",
	)
	if err != nil {
		return Cleanup{}, err
	}
	settings.BatchSize, err = positiveInt("CLEANUP_BATCH_SIZE", 1000)
	if err != nil {
		return Cleanup{}, err
	}
	return settings, nil
}

func validateHTTPURL(name, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("%s must be an absolute HTTP(S) URL", name)
	}
	return nil
}

func positiveDuration(key, fallback string) (time.Duration, error) {
	result, err := time.ParseDuration(value(key, fallback))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	if result <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return result, nil
}

func positiveInt(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	result, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	if result <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return result, nil
}

func value(key, fallback string) string {
	if result := os.Getenv(key); result != "" {
		return result
	}
	return fallback
}

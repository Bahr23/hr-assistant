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
	DatabaseURL        string
	RabbitMQURL        string
	TelegramBotToken   string
	TelegramAPIBaseURL string
	OpenRouterAPIKey   string
	OpenRouterBaseURL  string
	OpenRouterModel    string
	EvaluationModel    string
	ReportModel        string
	HTTPAddress        string
	TempDir            string
	Concurrency        int
	HTTPTimeout        time.Duration
	LeaseDuration      time.Duration
	MaxBackoff         time.Duration
	CleanupInterval    time.Duration
	InboxRetention     time.Duration
	OutboxRetention    time.Duration
	JobRetention       time.Duration
}

type Migrate struct {
	DatabaseURL   string
	MigrationsDir string
}

type Topology struct{ RabbitMQURL string }

type Healthcheck struct {
	URL     string
	Timeout time.Duration
}

func LoadServe() (Serve, error) {
	settings := Serve{
		DatabaseURL: os.Getenv("DATABASE_URL"), RabbitMQURL: os.Getenv("RABBITMQ_URL"),
		TelegramBotToken: os.Getenv("BOT_TOKEN"), OpenRouterAPIKey: os.Getenv("OPENROUTER_API_KEY"),
		TelegramAPIBaseURL: strings.TrimRight(value("TELEGRAM_API_BASE_URL", "https://api.telegram.org"), "/"),
		OpenRouterBaseURL:  strings.TrimRight(value("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"), "/"),
		OpenRouterModel:    value("OPENROUTER_MODEL", "openai/gpt-4o-mini-transcribe"),
		EvaluationModel:    value("OPENROUTER_EVALUATION_MODEL", "openai/gpt-4.1-mini"),
		ReportModel:        value("OPENROUTER_REPORT_MODEL", "openai/gpt-5.6-terra"),
		HTTPAddress:        value("OPENROUTER_WORKER_HTTP_ADDRESS", ":8081"),
		TempDir:            value("OPENROUTER_TEMP_DIR", "/tmp/openrouter-worker"),
	}
	for name, raw := range map[string]string{
		"DATABASE_URL": settings.DatabaseURL, "RABBITMQ_URL": settings.RabbitMQURL,
		"BOT_TOKEN": settings.TelegramBotToken, "OPENROUTER_API_KEY": settings.OpenRouterAPIKey,
	} {
		if strings.TrimSpace(raw) == "" {
			return Serve{}, fmt.Errorf("%s is required", name)
		}
	}
	if err := validateURL("RABBITMQ_URL", settings.RabbitMQURL, "amqp", "amqps"); err != nil {
		return Serve{}, err
	}
	if err := validateURL("TELEGRAM_API_BASE_URL", settings.TelegramAPIBaseURL, "http", "https"); err != nil {
		return Serve{}, err
	}
	if err := validateURL("OPENROUTER_BASE_URL", settings.OpenRouterBaseURL, "http", "https"); err != nil {
		return Serve{}, err
	}
	if _, _, err := net.SplitHostPort(settings.HTTPAddress); err != nil {
		return Serve{}, fmt.Errorf("OPENROUTER_WORKER_HTTP_ADDRESS: %w", err)
	}
	var err error
	settings.Concurrency, err = positiveInt("OPENROUTER_WORKER_CONCURRENCY", 4)
	if err != nil {
		return Serve{}, err
	}
	settings.HTTPTimeout, err = positiveDuration("OPENROUTER_HTTP_TIMEOUT", "5m")
	if err != nil {
		return Serve{}, err
	}
	settings.LeaseDuration, err = positiveDuration("OPENROUTER_JOB_LEASE", "10m")
	if err != nil {
		return Serve{}, err
	}
	settings.MaxBackoff, err = positiveDuration("OPENROUTER_MAX_BACKOFF", "30m")
	if err != nil {
		return Serve{}, err
	}
	settings.CleanupInterval, err = positiveDuration("OPENROUTER_CLEANUP_INTERVAL", "1h")
	if err != nil {
		return Serve{}, err
	}
	settings.InboxRetention, err = positiveDuration("OPENROUTER_INBOX_RETENTION", "720h")
	if err != nil {
		return Serve{}, err
	}
	settings.OutboxRetention, err = positiveDuration("OPENROUTER_OUTBOX_RETENTION", "168h")
	if err != nil {
		return Serve{}, err
	}
	settings.JobRetention, err = positiveDuration("OPENROUTER_JOB_RETENTION", "2160h")
	if err != nil {
		return Serve{}, err
	}
	return settings, nil
}

func LoadMigrate() (Migrate, error) {
	settings := Migrate{DatabaseURL: os.Getenv("DATABASE_URL"), MigrationsDir: value("MIGRATIONS_DIR", "migrations")}
	if settings.DatabaseURL == "" {
		return Migrate{}, fmt.Errorf("DATABASE_URL is required")
	}
	return settings, nil
}

func LoadTopology() (Topology, error) {
	settings := Topology{RabbitMQURL: os.Getenv("RABBITMQ_URL")}
	if settings.RabbitMQURL == "" {
		return Topology{}, fmt.Errorf("RABBITMQ_URL is required")
	}
	if err := validateURL("RABBITMQ_URL", settings.RabbitMQURL, "amqp", "amqps"); err != nil {
		return Topology{}, err
	}
	return settings, nil
}

func LoadHealthcheck() (Healthcheck, error) {
	timeout, err := positiveDuration("HEALTHCHECK_TIMEOUT", "2s")
	if err != nil {
		return Healthcheck{}, err
	}
	return Healthcheck{URL: value("OPENROUTER_WORKER_HEALTHCHECK_URL", "http://127.0.0.1:8081/health/ready"), Timeout: timeout}, nil
}

func validateURL(name, raw string, schemes ...string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%s must be an absolute URL", name)
	}
	for _, scheme := range schemes {
		if parsed.Scheme == scheme {
			return nil
		}
	}
	return fmt.Errorf("%s has an unsupported URL scheme", name)
}

func positiveDuration(name, fallback string) (time.Duration, error) {
	result, err := time.ParseDuration(value(name, fallback))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if result <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return result, nil
}

func positiveInt(name string, fallback int) (int, error) {
	raw := value(name, strconv.Itoa(fallback))
	result, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if result <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return result, nil
}

func value(name, fallback string) string {
	if result := os.Getenv(name); result != "" {
		return result
	}
	return fallback
}

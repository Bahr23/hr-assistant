package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMigrateConfigurationDoesNotRequireServeSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db/example")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("FRONTEND_ORIGIN", "not-a-url")

	settings, err := LoadMigrate()

	require.NoError(t, err)
	require.Equal(t, "postgres://db/example", settings.DatabaseURL)
}

func TestHealthcheckUsesConfiguredHTTPAddress(t *testing.T) {
	t.Setenv("HEALTHCHECK_URL", "")
	t.Setenv("HTTP_ADDRESS", ":9097")

	settings, err := LoadHealthcheck()

	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:9097/health/ready", settings.ReadinessURL)
}

func TestServeValidatesOnlyServeConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db/example")
	t.Setenv("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/")
	t.Setenv("JWT_SECRET", "short")

	_, err := LoadServe()

	require.EqualError(t, err, "JWT_SECRET must contain at least 32 characters")
}

func TestServeRequiresMasterAPIToken(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db/example")
	t.Setenv("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/")
	t.Setenv("JWT_SECRET", "this-jwt-secret-is-at-least-32-characters")
	t.Setenv("MASTER_API_TOKEN", "short")

	_, err := LoadServe()

	require.EqualError(t, err, "MASTER_API_TOKEN must contain at least 32 characters")
}

func TestServeRequiresRabbitMQURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db/example")
	t.Setenv("RABBITMQ_URL", "")

	_, err := LoadServe()

	require.EqualError(t, err, "RABBITMQ_URL is required")
}

func TestTelegramBotURLMustUseTelegramHTTPSLink(t *testing.T) {
	require.NoError(t, validateTelegramBotURL("https://t.me/bot"))
	require.EqualError(
		t,
		validateTelegramBotURL("http://example.com/bot"),
		"TELEGRAM_BOT_URL must be an absolute https://t.me/<bot> URL",
	)
}

func TestCleanupUsesSafeDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db/example")
	t.Setenv("CLEANUP_INTERVAL", "")
	t.Setenv("CLEANUP_BATCH_SIZE", "")
	t.Setenv("CLEANUP_INBOX_RETENTION", "")
	t.Setenv("CLEANUP_OUTBOX_RETENTION", "")
	t.Setenv("CLEANUP_TELEGRAM_DELIVERY_RETENTION", "")

	settings, err := LoadCleanup()

	require.NoError(t, err)
	require.Equal(t, time.Hour, settings.Interval)
	require.Equal(t, 1000, settings.BatchSize)
	require.Equal(t, 30*24*time.Hour, settings.InboxRetention)
	require.Equal(t, 7*24*time.Hour, settings.OutboxRetention)
	require.Equal(t, 90*24*time.Hour, settings.TelegramDeliveryRetention)
}

func TestCleanupRejectsInvalidBatchSize(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db/example")
	t.Setenv("CLEANUP_BATCH_SIZE", "0")

	_, err := LoadCleanup()

	require.EqualError(t, err, "CLEANUP_BATCH_SIZE must be positive")
}

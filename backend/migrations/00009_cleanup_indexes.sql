-- +goose Up
CREATE INDEX inbox_messages_received_at_idx
    ON inbox_messages(received_at);

CREATE INDEX outbox_events_published_at_idx
    ON outbox_events(published_at)
    WHERE published_at IS NOT NULL;

CREATE INDEX telegram_deliveries_terminal_updated_at_idx
    ON telegram_deliveries(updated_at)
    WHERE status IN ('sent', 'failed');

-- +goose Down
DROP INDEX IF EXISTS telegram_deliveries_terminal_updated_at_idx;
DROP INDEX IF EXISTS outbox_events_published_at_idx;
DROP INDEX IF EXISTS inbox_messages_received_at_idx;

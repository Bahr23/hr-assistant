package repository

import (
	"time"

	"hr-assistant/backend/internal/core/database/dbgen"
	"hr-assistant/backend/internal/core/domain"

	"github.com/jackc/pgx/v5/pgtype"
)

func userFromDB(value dbgen.User) domain.User {
	return domain.User{
		ID: value.ID, Email: value.Email,
		PasswordHash: value.PasswordHash, CreatedAt: value.CreatedAt.Time, UpdatedAt: value.UpdatedAt.Time,
	}
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

package httpapi

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresAuthStore struct {
	db *pgxpool.Pool
}

func NewPostgresAuthStore(db *pgxpool.Pool) *PostgresAuthStore {
	return &PostgresAuthStore{db: db}
}

func (s *PostgresAuthStore) AuthenticateAPIKey(ctx context.Context, key string) (APIKeyPrincipal, error) {
	keyHash := hashAPIKey(key)

	var principal APIKeyPrincipal
	var storedHash string
	if err := s.db.QueryRow(ctx, `
		SELECT id, name, role, key_hash
		FROM api_keys
		WHERE key_hash = $1
		  AND active = TRUE
		  AND revoked_at IS NULL
	`, keyHash).Scan(&principal.ID, &principal.Name, &principal.Role, &storedHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return APIKeyPrincipal{}, errInvalidAPIKey
		}
		return APIKeyPrincipal{}, err
	}

	if !constantTimeStringEqual(storedHash, keyHash) {
		return APIKeyPrincipal{}, errInvalidAPIKey
	}
	return principal, nil
}

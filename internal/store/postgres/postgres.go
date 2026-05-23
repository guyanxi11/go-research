package postgres

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourname/go-research/internal/config"
	"github.com/yourname/go-research/internal/store"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Store implements store.ResearchStore on PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

const (
	pgConnectAttempts = 20
	pgConnectInterval = 500 * time.Millisecond
)

// Open connects, runs embedded migrations, and returns a ready Store.
// Retries ping for a few seconds so `make up` immediately followed by
// `make run` works while Postgres is still starting inside Docker.
func Open(ctx context.Context, cfg config.PostgresConfig) (*Store, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("postgres connect: %w", err)
	}

	var pingErr error
	for attempt := 1; attempt <= pgConnectAttempts; attempt++ {
		pingErr = pool.Ping(ctx)
		if pingErr == nil {
			break
		}
		if attempt == pgConnectAttempts {
			pool.Close()
			return nil, fmt.Errorf(
				"postgres ping after %d attempts: %w\n(hint: run `make up` and wait until healthy, or check POSTGRES_* in .env)",
				pgConnectAttempts, pingErr,
			)
		}
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(pgConnectInterval):
		}
	}

	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *Store) migrate(ctx context.Context) error {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := s.pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}

// Ensure Store implements ResearchStore.
var _ store.ResearchStore = (*Store)(nil)

// Ping is used by health checks.
func (s *Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.pool.Ping(ctx)
}

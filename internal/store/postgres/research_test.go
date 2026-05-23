package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/yourname/go-research/internal/config"
)

// Integration test: requires `make up` and existing session row.
func TestGetSession_NullErrorMessage(t *testing.T) {
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to run against local Postgres")
	}
	ctx := context.Background()
	st, err := Open(ctx, config.PostgresConfig{
		Host:     envOr("POSTGRES_HOST", "127.0.0.1"),
		Port:     5432,
		User:     envOr("POSTGRES_USER", "research"),
		Password: envOr("POSTGRES_PASSWORD", "research"),
		DB:       envOr("POSTGRES_DB", "research"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	list, err := st.ListSessions(ctx, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Skip("no sessions in db")
	}
	det, err := st.GetSession(ctx, list[0].ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if det == nil {
		t.Fatal("expected session detail")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

package backend

import (
	"context"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"whats-gtk/internal/logger"
)

func InitStore(ctx context.Context, dbPath string) (*sqlstore.Container, error) {
	dbLog := logger.WALogger("Database")
	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbPath), dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to store: %w", err)
	}
	return container, nil
}

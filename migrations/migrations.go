package migrations

import (
	"context"
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

func Up(ctx context.Context, db *sql.DB) error {
	p, err := goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		return err
	}
	_, err = p.Up(ctx)
	return err
}

// Down rolls back the most recent migration.
func Down(ctx context.Context, db *sql.DB) error {
	p, err := goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		return err
	}
	_, err = p.Down(ctx)
	return err
}

// DownTo rolls back to an explicit migration version; integration tests use zero on their empty disposable databases.
func DownTo(ctx context.Context, db *sql.DB, version int64) error {
	p, err := goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		return err
	}
	_, err = p.DownTo(ctx, version)
	return err
}

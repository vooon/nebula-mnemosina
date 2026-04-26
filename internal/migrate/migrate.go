package migrate

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tern "github.com/jackc/tern/v2/migrate"

	"github.com/vooon/nebula-mnemosina/db/migrations"
)

type Options struct {
	EnableTimescale bool
}

func Run(ctx context.Context, pool *pgxpool.Pool, options Options) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	if err := runSet(ctx, conn.Conn(), "public.mnemo_schema_version", "core"); err != nil {
		return err
	}
	if options.EnableTimescale {
		if err := runSet(ctx, conn.Conn(), "public.mnemo_timescale_version", "timescale"); err != nil {
			return err
		}
	}
	if err := runSet(ctx, conn.Conn(), "public.mnemo_views_version", "views"); err != nil {
		return err
	}

	return nil
}

func runSet(ctx context.Context, conn *pgx.Conn, versionTable, dir string) error {
	fsys, err := fs.Sub(migrations.Files, dir)
	if err != nil {
		return fmt.Errorf("open embedded %s migrations: %w", dir, err)
	}

	migrator, err := tern.NewMigrator(ctx, conn, versionTable)
	if err != nil {
		return fmt.Errorf("initialize %s migrator: %w", dir, err)
	}
	if err := migrator.LoadMigrations(fsys); err != nil {
		return fmt.Errorf("load %s migrations: %w", dir, err)
	}
	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("run %s migrations: %w", dir, err)
	}
	return nil
}

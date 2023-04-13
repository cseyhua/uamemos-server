package db

import (
	"context"
	"database/sql"
	"strings"
)

type MigrationHistory struct {
	Version   string
	CreatedTs int64
}

type MigrationHistoryUpsert struct {
	Version string
}

type MigrationHistoryFind struct {
	Version *string
}

func (db *DB) FindMigrationHistoryList(ctx context.Context, find *MigrationHistoryFind) ([]*MigrationHistory, error) {
	tx, err := db.DBInstance.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	list, err := findMigrationHistoryList(ctx, tx, find)
	if err != nil {
		return nil, err
	}
	return list, nil
}

func findMigrationHistoryList(ctx context.Context, tx *sql.Tx, find *MigrationHistoryFind) ([]*MigrationHistory, error) {
	where, args := []string{"1 = 1"}, []any{}
	// 版本没有指定，则查找所有版本
	if v := find.Version; v != nil {
		where, args = append(where, "version = ?"), append(args, *v)
	}
	query := `
		SELECT version,createdTs
		FROM migration_history
		WHERE` + strings.Join(where, " AND ") + `
		ORDER BY version DESC
	`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	migrationHistoryList := make([]*MigrationHistory, 0)
	for rows.Next() {
		var migrationHistory MigrationHistory
		if err := rows.Scan(&migrationHistory.Version, &migrationHistory.CreatedTs); err != nil {
			return nil, err
		}
		migrationHistoryList = append(migrationHistoryList, &migrationHistory)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return migrationHistoryList, nil
}

func (db *DB) UpsertMigrationHistory(ctx context.Context, upsert *MigrationHistoryUpsert) (*MigrationHistory, error) {
	tx, err := db.DBInstance.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	migrationHistory, err := upsertMigrationHistory(ctx, tx, upsert)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return migrationHistory, nil
}

func upsertMigrationHistory(ctx context.Context, tx *sql.Tx, upsert *MigrationHistoryUpsert) (*MigrationHistory, error) {
	query := `
		INSERT INTO migration_history ( version )
		VALUES (?)
		ON CONFLICT(version) DO UPDATE
		SET version=EXCLUDED.version
		RETURNING version, created_ts
	`
	var migrationHistory MigrationHistory
	if err := tx.QueryRowContext(ctx, query, upsert.Version).Scan(&migrationHistory.Version, &migrationHistory.CreatedTs); err != nil {
		return nil, err
	}
	return &migrationHistory, nil
}

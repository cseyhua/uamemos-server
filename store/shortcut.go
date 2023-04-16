package store

import (
	"context"
	"database/sql"
)

func vacuumShortcut(ctx context.Context, tx *sql.Tx) error {
	stmt := `
	DELETE FROM 
		shortcut 
	WHERE 
		creator_id NOT IN (
			SELECT 
				id 
			FROM 
				user
		)`
	_, err := tx.ExecContext(ctx, stmt)
	if err != nil {
		return FormatError(err)
	}

	return nil
}

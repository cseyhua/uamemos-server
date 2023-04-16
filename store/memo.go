package store

import (
	"context"
	"database/sql"
)

func vacuumMemo(ctx context.Context, tx *sql.Tx) error {
	stmt := `
	DELETE FROM 
		memo 
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

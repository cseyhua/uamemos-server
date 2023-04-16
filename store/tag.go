package store

import (
	"context"
	"database/sql"
)

func vacuumTag(ctx context.Context, tx *sql.Tx) error {
	stmt := `
	DELETE FROM 
		tag 
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

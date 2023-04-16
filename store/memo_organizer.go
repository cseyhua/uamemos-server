package store

import (
	"context"
	"database/sql"
)

func vacuumMemoOrganizer(ctx context.Context, tx *sql.Tx) error {
	stmt := `
	DELETE FROM 
		memo_organizer 
	WHERE 
		memo_id NOT IN (
			SELECT 
				id 
			FROM 
				memo
		)
		OR user_id NOT IN (
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

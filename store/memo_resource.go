package store

import (
	"context"
	"database/sql"
)

func vacuumMemoResource(ctx context.Context, tx *sql.Tx) error {
	stmt := `
	DELETE FROM 
		memo_resource 
	WHERE 
		memo_id NOT IN (
			SELECT 
				id 
			FROM 
				memo
		) 
		OR resource_id NOT IN (
			SELECT 
				id 
			FROM 
				resource
		)`
	_, err := tx.ExecContext(ctx, stmt)
	if err != nil {
		return FormatError(err)
	}

	return nil
}

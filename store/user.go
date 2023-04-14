package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"uamemos/api"
	"uamemos/common"
)

type userRaw struct {
	ID int

	// Standard fields
	RowStatus api.RowStatus
	CreatedTs int64
	UpdatedTs int64

	// Domain specific fields
	Name         string
	Role         api.Role
	Email        string
	Nickname     string
	PasswordHash string
	OpenID       string
	AvatarURL    string
}

func (raw *userRaw) toUser() *api.User {
	return &api.User{
		ID: raw.ID,

		RowStatus: raw.RowStatus,
		CreatedTs: raw.CreatedTs,
		UpdatedTs: raw.UpdatedTs,

		Name:         raw.Name,
		Role:         raw.Role,
		Email:        raw.Email,
		Nickname:     raw.Nickname,
		PasswordHash: raw.PasswordHash,
		OpenID:       raw.OpenID,
		AvatarURL:    raw.AvatarURL,
	}
}

func (s *Store) CreateUser(ctx context.Context, create *api.UserCreate) (*api.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, FormatError(err)
	}
	defer tx.Rollback()

	userRaw, err := createUser(ctx, tx, create)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, FormatError(err)
	}

	s.userCache.Store(userRaw.ID, userRaw)
	user := userRaw.toUser()
	return user, nil
}

func createUser(ctx context.Context, tx *sql.Tx, create *api.UserCreate) (*userRaw, error) {
	query := `
		INSERT INTO user (
			username,
			role,
			email,
			nickname,
			password_hash,
			open_id
		)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id, username, role, email, nickname, password_hash, open_id, avatar_url, created_ts, updated_ts, row_status
	`
	var userRaw userRaw
	if err := tx.QueryRowContext(ctx, query,
		create.Name,
		create.Role,
		create.Email,
		create.Nickname,
		create.PasswordHash,
		create.OpenID,
	).Scan(
		&userRaw.ID,
		&userRaw.Name,
		&userRaw.Role,
		&userRaw.Email,
		&userRaw.Nickname,
		&userRaw.PasswordHash,
		&userRaw.OpenID,
		&userRaw.AvatarURL,
		&userRaw.CreatedTs,
		&userRaw.UpdatedTs,
		&userRaw.RowStatus,
	); err != nil {
		return nil, FormatError(err)
	}

	return &userRaw, nil
}

func (s *Store) FindUserList(ctx context.Context, find *api.UserFind) ([]*api.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, FormatError(err)
	}
	defer tx.Rollback()

	userRawList, err := findUserList(ctx, tx, find)
	if err != nil {
		return nil, err
	}

	list := []*api.User{}
	for _, raw := range userRawList {
		list = append(list, raw.toUser())
	}

	return list, nil
}

func (s *Store) FindUser(ctx context.Context, find *api.UserFind) (*api.User, error) {
	if find.ID != nil {
		if user, ok := s.userCache.Load(*find.ID); ok {
			return user.(*userRaw).toUser(), nil
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, FormatError(err)
	}
	defer tx.Rollback()

	list, err := findUserList(ctx, tx, find)
	if err != nil {
		return nil, err
	}

	if len(list) == 0 {
		return nil, &common.Error{Code: common.NotFound, Err: fmt.Errorf("not found user with filter %+v", find)}
	}

	userRaw := list[0]
	s.userCache.Store(userRaw.ID, userRaw)
	user := userRaw.toUser()
	return user, nil
}

func findUserList(ctx context.Context, tx *sql.Tx, find *api.UserFind) ([]*userRaw, error) {
	where, args := []string{"1 = 1"}, []any{}

	if v := find.ID; v != nil {
		where, args = append(where, "id = ?"), append(args, *v)
	}
	if v := find.Name; v != nil {
		where, args = append(where, "username = ?"), append(args, *v)
	}
	if v := find.Role; v != nil {
		where, args = append(where, "role = ?"), append(args, *v)
	}
	if v := find.Email; v != nil {
		where, args = append(where, "email = ?"), append(args, *v)
	}
	if v := find.Nickname; v != nil {
		where, args = append(where, "nickname = ?"), append(args, *v)
	}
	if v := find.OpenID; v != nil {
		where, args = append(where, "open_id = ?"), append(args, *v)
	}

	query := `
		SELECT 
			id,
			username,
			role,
			email,
			nickname,
			password_hash,
			open_id,
			avatar_url,
			created_ts,
			updated_ts,
			row_status
		FROM user
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY created_ts DESC, row_status DESC
	`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, FormatError(err)
	}
	defer rows.Close()

	userRawList := make([]*userRaw, 0)
	for rows.Next() {
		var userRaw userRaw
		if err := rows.Scan(
			&userRaw.ID,
			&userRaw.Name,
			&userRaw.Role,
			&userRaw.Email,
			&userRaw.Nickname,
			&userRaw.PasswordHash,
			&userRaw.OpenID,
			&userRaw.AvatarURL,
			&userRaw.CreatedTs,
			&userRaw.UpdatedTs,
			&userRaw.RowStatus,
		); err != nil {
			return nil, FormatError(err)
		}
		userRawList = append(userRawList, &userRaw)
	}

	if err := rows.Err(); err != nil {
		return nil, FormatError(err)
	}

	return userRawList, nil
}

package store

import (
	"database/sql"
	"sync"
	"uamemos/service/profile"
)

type Store struct {
	db      *sql.DB
	profile *profile.Profile

	userCache        sync.Map // map[int]*userRaw
	userSettingCache sync.Map // map[string]*userSettingRaw
	memoCache        sync.Map // map[int]*memoRaw
	shortcutCache    sync.Map // map[int]*shortcutRaw
	idpCache         sync.Map // map[int]*identityProviderMessage
}

func New(db *sql.DB, profile *profile.Profile) *Store {
	return &Store{
		db:      db,
		profile: profile,
	}
}

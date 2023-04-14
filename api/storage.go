package api

const (
	// LocalStorage means the storage service is local file system.
	LocalStorage = -1
	// DatabaseStorage means the storage service is database.
	DatabaseStorage = 0
)

type StorageType string

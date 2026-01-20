package image

type Cache interface {
	// GetPath returns the absolute path for the given key
	GetPath(key string) string
	// Exists checks if the cache key exists and updates access time
	Exists(key string) bool
}

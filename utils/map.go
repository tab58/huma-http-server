package utils

// Keys returns the keys of the map as a slice
func Keys[T comparable](m map[T]any) []T {
	keys := make([]T, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

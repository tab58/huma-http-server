package utils

// Dedupe compares the values in the slice and returns a new slice with duplicates removed
func Dedupe[T comparable](slice []T) []T {
	deduped := make(map[T]any)
	for _, value := range slice {
		deduped[value] = struct{}{}
	}
	return Keys(deduped)
}

func Map[T any, R any](slice []T, fn func(T) R) []R {
	result := make([]R, len(slice))
	for i, v := range slice {
		result[i] = fn(v)
	}
	return result
}

func MapErr[T any, R any](slice []T, fn func(T) (R, error)) ([]R, error) {
	result := make([]R, len(slice))
	var err error
	for i, v := range slice {
		result[i], err = fn(v)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func Run[T func() error](slice ...T) error {
	for _, fn := range slice {
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}

func Filter[T any](slice []T, fn func(T) bool) []T {
	result := make([]T, 0)
	for _, v := range slice {
		if fn(v) {
			result = append(result, v)
		}
	}
	return result
}

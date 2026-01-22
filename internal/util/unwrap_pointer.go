package util

func UnwrapPointer[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}

func UnwrapPointerKey[K, V any](k *K, v V) (K, V) {
	return UnwrapPointer(k), v
}

func UnwrapPointerValue[K, V any](k K, v *V) (K, V) {
	return k, UnwrapPointer(v)
}

func UnwrapPointers[T any](s []*T) []T {
	if s == nil {
		return nil
	}
	var dst = make([]T, len(s))
	for index, v := range s {
		dst[index] = UnwrapPointer(v)
	}
	return dst
}

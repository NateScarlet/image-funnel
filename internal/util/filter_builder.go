package util

type FilterBuilder[T any] struct {
	s []func(T) bool
}

func (b FilterBuilder[T]) Build() func(T) bool {
	return func(v T) bool {
		for _, i := range b.s {
			if !i(v) {
				return false
			}
		}
		return true
	}
}

func (b *FilterBuilder[T]) Add(filter func(obj T) bool) {
	b.s = append(b.s, filter)
}

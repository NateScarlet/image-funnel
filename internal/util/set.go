package util

import (
	"iter"
	"slices"
	"sort"
)

type Set[T comparable] map[T]struct{}

func (s Set[T]) Remove(values ...T) {
	for _, i := range values {
		delete(s, i)
	}
}

func (s Set[T]) Clear() {
	for k := range s {
		delete(s, k)
	}
}

func (s Set[T]) Has(v T) bool {
	_, ok := s[v]
	return ok
}

// Seq returns underlying map keys.
//
// order is not guaranteed.
func (s Set[T]) Seq() func(func(T) bool) {
	return func(yield func(T) bool) {
		for i := range s {
			if !yield(i) {
				return
			}
		}
	}
}

// Values returns a new slice of all the values in the set.
//
// sorted with `less` func, nil means unspecified order as map keys.
func (s Set[T]) Values(less func(a, b T) bool) []T {
	var v = make([]T, 0, len(s))
	v = slices.AppendSeq(v, s.Seq())
	if len(v) > 1 && less != nil {
		sort.Slice(v, func(i, j int) bool {
			return less(v[i], v[j])
		})
	}
	return v
}

func (s Set[T]) IntersectSlice(slice []T) []T {
	var ret = make([]T, 0, min(len(s), len(slice)))
	for _, i := range slice {
		if s.Has(i) {
			ret = append(ret, i)
		}
	}
	return ret
}

func (s Set[T]) Equal(o Set[T]) bool {
	if len(s) != len(o) {
		return false
	}
	for i := range s {
		if !o.Has(i) {
			return false
		}
	}
	return true
}

func AddToSet[T comparable](m Set[T], s ...T) Set[T] {
	if len(s) == 0 {
		return m
	}
	if m == nil {
		m = make(Set[T], len(s))
	}
	for _, i := range s {
		m[i] = struct{}{}
	}
	return m
}

func InsertToSet[T comparable](dst Set[T], seq iter.Seq[T]) Set[T] {
	for i := range seq {
		if dst == nil {
			dst = make(Set[T])
		}
		dst[i] = struct{}{}
	}
	return dst
}

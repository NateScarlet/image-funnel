package enum

import (
	"fmt"
	"iter"
)

type Option[T any] func(*T)

type member[T any] struct {
	str  string
	meta T
}

// Class should not expose to other module.
type Class[T any] struct {
	objects    []member[T]
	indexByStr map[string]int
}

func (cls *Class[T]) meta(s string) T {
	if index, ok := cls.indexByStr[s]; ok {
		return cls.objects[index].meta
	}
	if s != "" {
		panic(fmt.Errorf("undefined enum value %q", s))
	}
	var zero T
	return zero
}

func (cls *Class[T]) Values() iter.Seq[Enum[T]] {
	return func(yield func(Enum[T]) bool) {
		for _, i := range cls.objects {
			if !yield(Enum[T]{s: i.str}) {
				return
			}
		}
	}
}

func (cls *Class[T]) parse(s string, dst *Enum[T]) (err error) {
	if index, ok := cls.indexByStr[s]; ok {
		dst.s = cls.objects[index].str
		return
	}
	err = fmt.Errorf("invalid input")
	return
}

// Parse implements Class.
func (cls *Class[T]) Parse(s string) (_ Enum[T], err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%T.Parse(%q): %w", cls, s, err)
		}
	}()
	var v = new(Enum[T])
	err = cls.parse(s, v)
	return *v, err
}

// Define implements Class.
func (cls *Class[T]) Define(s string, meta ...T) Enum[T] {
	if _, ok := cls.indexByStr[s]; ok {
		panic("already registered")
	}
	var m T
	if len(meta) > 1 {
		panic("multiple meta is not allowed")
	} else if len(meta) == 1 {
		m = meta[0]
	}
	var index = len(cls.objects)
	cls.objects = append(cls.objects, member[T]{s, m})
	cls.indexByStr[s] = index
	return Enum[T]{s}
}

func (cls *Class[T]) DefineAlias(alias string, target string) Enum[T] {
	if _, ok := cls.indexByStr[alias]; ok {
		panic("already registered")
	}
	var index, ok = cls.indexByStr[target]
	if !ok {
		panic("alias target is not registered")
	}
	cls.indexByStr[alias] = index
	return Enum[T]{cls.objects[index].str}
}

func New[T any]() (_ *Class[T]) {
	var key = registryKey[T]()
	if _, ok := registry.m[key]; ok {
		panic("meta type already been used")
	}
	var cls = &Class[T]{
		indexByStr: make(map[string]int),
	}
	registry.m[key] = cls
	return cls
}

func Parse[T any](s string) (_ Enum[T], err error) {
	return classOf[T]().Parse(s)
}

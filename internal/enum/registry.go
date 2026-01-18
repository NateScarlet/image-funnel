package enum

import (
	"reflect"
)

var registry = struct {
	m map[reflect.Type]any
}{
	m: make(map[reflect.Type]any),
}

func registryKey[T any]() reflect.Type {
	return reflect.TypeOf((*T)(nil)).Elem()
}

func classOf[T any]() *Class[T] {
	return registry.m[registryKey[T]()].(*Class[T])
}

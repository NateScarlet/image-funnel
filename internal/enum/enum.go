package enum

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/99designs/gqlgen/graphql"
)

type Enum[T any] struct {
	s string
}

// IsZero implements Enum.
func (e Enum[T]) IsZero() bool {
	return e.s == ""
}

// String implements Enum.
func (e Enum[T]) String() string {
	return e.s
}

func (e Enum[T]) Meta() T {
	return classOf[T]().meta(e.s)
}

func (e Enum[T]) GoString() string {
	return fmt.Sprintf("%T(%q)", e, e.s)
}

var _ json.Marshaler = Enum[any]{}
var _ json.Unmarshaler = (*Enum[any])(nil)

// MarshalJSON implements Enum.
func (obj Enum[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(obj.String())
}

// UnmarshalJSON implements Enum.
func (obj *Enum[T]) UnmarshalJSON(data []byte) (err error) {
	var s string
	err = json.Unmarshal(data, &s)
	if err != nil {
		return
	}
	err = classOf[T]().parse(s, obj)
	return
}

var _ graphql.Marshaler = Enum[any]{}

var _ graphql.Unmarshaler = (*Enum[any])(nil)

// MarshalGQL implements Enum.
func (obj Enum[T]) MarshalGQL(w io.Writer) {
	graphql.MarshalString(obj.String()).MarshalGQL(w)
}

// UnmarshalGQL implements Enum.
func (obj *Enum[T]) UnmarshalGQL(v interface{}) (err error) {
	s, err := graphql.UnmarshalString(v)
	if err != nil {
		return
	}
	err = classOf[T]().parse(s, obj)
	return
}

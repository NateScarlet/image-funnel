package scalar

import (
	"fmt"
	"io"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
)

func ToID(str string) ID {
	return ID{str: str}
}

func NewID() ID {
	return ID{str: uuid.NewString()}
}

func ParseID(str string) (ID, error) {
	return ID{str: str}, nil // 占位，以后可能会出错
}

type ID struct{ str string }

func (id ID) String() string {
	return id.str
}

func (id ID) IsZero() bool {
	return id.str == ""
}

var _ graphql.Marshaler = ID{}
var _ graphql.Unmarshaler = (*ID)(nil)

func (id ID) MarshalGQL(w io.Writer) {
	graphql.MarshalString(id.str).MarshalGQL(w)
}

func (id *ID) UnmarshalGQL(v interface{}) error {
	switch v := v.(type) {
	case string:
		var err error
		*id, err = ParseID(v)
		return err
	default:
		return fmt.Errorf("unexpected ID: %v", v)
	}
}

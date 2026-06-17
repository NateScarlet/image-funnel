package scalar

import (
	"fmt"
	"io"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
)

// ToID 将字符串转换为 ID 类型
// 注意：仅供领域层使用，外部必须直接原样传递ID类型，避免字符串误用
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

// MarshalText 实现 encoding.TextMarshaler 接口以支持 TOML 序列化
func (id ID) MarshalText() ([]byte, error) {
	return []byte(id.str), nil
}

// UnmarshalText 实现 encoding.TextUnmarshaler 接口以支持 TOML 反序列化
func (id *ID) UnmarshalText(text []byte) error {
	id.str = string(text)
	return nil
}

// MarshalJSON 实现 json.Marshaler 接口
func (id ID) MarshalJSON() ([]byte, error) {
	return fmt.Appendf(nil, "%q", id.str), nil
}

// UnmarshalJSON 实现 json.Unmarshaler 接口
func (id *ID) UnmarshalJSON(data []byte) error {
	if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
		id.str = string(data[1 : len(data)-1])
		return nil
	}
	id.str = string(data)
	return nil
}

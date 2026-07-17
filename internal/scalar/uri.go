package scalar

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/99designs/gqlgen/graphql"
)

// URI 表示符合 RFC3986 规范的绝对或相对 URI。
type URI struct {
	u url.URL
}

// UnmarshalJSON 实现 json.Unmarshaler 接口
func (u *URI) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	*u, err = ParseURI(s)
	return err
}

// MarshalJSON 实现 json.Marshaler 接口
func (u URI) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

func (uri URI) IsAbs() bool {
	return uri.u.IsAbs()
}

func (uri URI) Scheme() string {
	return uri.u.Scheme
}

func (uri URI) Path() string {
	return uri.u.Path
}

func (uri URI) Host() string {
	return uri.u.Host
}

func (uri URI) Opaque() string {
	return uri.u.Opaque
}

func newURI(u url.URL) URI {
	u.RawPath = ""
	u.RawFragment = ""
	u.User = nil
	return URI{u}
}

func FromURL(u *url.URL) URI {
	if u == nil {
		return URI{}
	}
	return newURI(*u)
}

// UnmarshalGQL 实现 graphql.Unmarshaler 接口
func (uri *URI) UnmarshalGQL(v interface{}) (err error) {
	if v == nil {
		*uri = URI{}
		return
	}
	s, err := graphql.UnmarshalString(v)
	if err != nil {
		return
	}
	*uri, err = ParseURI(s)
	return
}

// MarshalGQL 实现 graphql.Marshaler 接口
func (uri URI) MarshalGQL(w io.Writer) {
	graphql.MarshalString(uri.String()).MarshalGQL(w)
}

func (uri URI) IsZero() bool {
	return uri.String() == ""
}

func (uri URI) URL() *url.URL {
	return &uri.u
}

func (uri URI) String() string {
	return uri.u.String()
}

func (uri URI) Equal(other URI) bool {
	return uri.String() == other.String()
}

// ParseURI 将字符串解析为 URI。如果字符串不带协议 scheme 则报错拦截（保证其格式合规）
func ParseURI(s string) (_ URI, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("ParseURI(%q): %w", s, err)
		}
	}()

	if s == "" {
		return
	}
	u, err := url.Parse(s)
	if err != nil {
		return
	}

	return newURI(*u), nil
}

func MustParseURI(s string) (_ URI) {
	v, err := ParseURI(s)
	if err != nil {
		panic(err)
	}
	return v
}

var _ json.Marshaler = URI{}
var _ json.Unmarshaler = (*URI)(nil)
var _ graphql.Marshaler = URI{}
var _ graphql.Unmarshaler = (*URI)(nil)

package graph

import (
	"context"
	"io"
	"strconv"
	"time"

	"github.com/99designs/gqlgen/graphql"
)

func MarshalTime(v time.Time) graphql.ContextMarshaler {
	return graphql.ContextWriterFunc(func(ctx context.Context, w io.Writer) (err error) {
		if v.IsZero() {
			var nullable bool = true
			if fCtx := graphql.GetFieldContext(ctx); fCtx != nil {
				nullable = !fCtx.Field.Definition.Type.NonNull
			}
			if nullable {
				return graphql.Null.MarshalGQLContext(ctx, w)
			}
		}
		_, err = io.WriteString(w, strconv.Quote(v.Format(time.RFC3339Nano)))
		return
	})
}

func UnmarshalTime(ctx context.Context, v interface{}) (ret time.Time, err error) {
	if v == "" {
		return
	}
	if v == nil {
		return
	}
	return graphql.UnmarshalTime(v)
}

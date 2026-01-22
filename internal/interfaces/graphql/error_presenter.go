package graphql

import (
	"context"
	"errors"
	"main/internal/apperror"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

var ErrorPresenter graphql.ErrorPresenterFunc = func(ctx context.Context, err error) *gqlerror.Error {
	{
		var v *apperror.AppError
		if apperror.As(err, &v) {
			err = v
		}
	}
	{
		var v interface{ GQLError() *gqlerror.Error }
		if errors.As(err, &v) {
			var ret = v.GQLError()
			ret.Path = graphql.GetPath(ctx)
			return ret
		}
	}
	return graphql.DefaultErrorPresenter(ctx, err)
}

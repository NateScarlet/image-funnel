// Package apperror contains app specific error
package apperror

import (
	"errors"

	"github.com/vektah/gqlparser/v2/gqlerror"
)

type Locales struct {
	Zh string `json:"zh,omitempty"`
}

func (l Locales) IsZero() bool {
	return l == Locales{}
}

// AppError that defined in application scope.
type AppError struct {
	Code       string
	Message    string
	Locales    Locales
	Extensions map[string]any
}

type Option = func(opts *AppError)

func WithExtension(key string, value any) Option {
	return func(opts *AppError) {
		if opts.Extensions == nil {
			opts.Extensions = make(map[string]any)
		}
		opts.Extensions[key] = value
	}
}

func New(
	code string,
	messageEn string,
	messageZh string,
	options ...Option,
) (obj *AppError) {
	obj = &AppError{
		Code:    code,
		Message: messageEn,
		Locales: Locales{
			Zh: messageZh,
		},
	}
	for _, i := range options {
		i(obj)
	}
	return
}

func (e *AppError) Error() string {
	return e.Message
}

// GQLError from app error
func (e AppError) GQLError() *gqlerror.Error {
	var extensions = make(map[string]any)
	for k, v := range e.Extensions {
		extensions[k] = v
	}
	if !e.Locales.IsZero() {
		extensions["locales"] = e.Locales
	}
	extensions["code"] = e.Code
	return &gqlerror.Error{
		Message:    e.Message,
		Extensions: extensions,
	}
}

// ErrTimeout when request timeout.
var ErrTimeout = &AppError{
	Message: "request timeout",
	Code:    "TIMEOUT",
	Locales: Locales{
		Zh: "请求超时",
	},
}

// ErrCode returns code for error, empty string when not found.
func ErrCode(err error) string {
	if err == nil {
		return ""
	}
	var appErr *AppError
	if As(err, &appErr) {
		return appErr.Code
	}
	var gqlErr *gqlerror.Error
	if errors.As(err, &gqlErr) {
		if code, ok := gqlErr.Extensions["code"]; ok {
			if v, ok := code.(string); ok {
				return v
			}
		}
	}
	return ""
}

package image

import (
	"fmt"
	"main/internal/scalar"
	"net/url"
)

type SignOption func(url.Values)

func WithWidth(w int) SignOption {
	return func(v url.Values) {
		v.Set("w", fmt.Sprintf("%d", w))
	}
}

func WithRaw() SignOption {
	return func(v url.Values) {
		v.Set("raw", "")
	}
}

type URLSigner interface {
	GenerateSignedURL(absPath string, opts ...SignOption) (scalar.URI, error)
}
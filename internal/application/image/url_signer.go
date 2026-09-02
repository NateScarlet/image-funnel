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

func WithQuality(q int) SignOption {
	return func(v url.Values) {
		v.Set("q", fmt.Sprintf("%d", q))
	}
}

func WithRaw() SignOption {
	return func(v url.Values) {
		v.Set("raw", "")
	}
}

func WithFormat(f ImageFormat) SignOption {
	return func(v url.Values) {
		v.Set("fmt", f.String())
	}
}

type URLSigner interface {
	GenerateSignedURL(absPath string, opts ...SignOption) (scalar.URI, error)
}

package image

import (
	"fmt"
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

type URLSigner interface {
	GenerateSignedURL(path string, opts ...SignOption) (string, error)
}

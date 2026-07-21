package session

import (
	"main/internal/scalar"
	"net/url"
)

type URLSigner interface {
	GenerateSignedURL(path string, extraParams ...url.Values) (scalar.URI, error)
}

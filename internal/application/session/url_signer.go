package session

import "net/url"

type URLSigner interface {
	GenerateSignedURL(path string, extraParams ...url.Values) (string, error)
}

package session

type URLSigner interface {
	GenerateSignedURL(path string) (string, error)
}

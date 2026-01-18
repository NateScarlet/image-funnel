package image

type URLSigner interface {
	GenerateSignedURL(path string) (string, error)
}

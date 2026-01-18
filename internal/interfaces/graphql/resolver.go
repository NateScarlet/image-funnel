//go:generate go tool github.com/99designs/gqlgen

package graphql

import (
	"main/internal/application"
	"main/internal/infrastructure/urlconv"
)

type Resolver struct {
	app     *application.Root
	rootDir string
	signer  *urlconv.Signer
	version string
}

func NewResolver(app *application.Root, rootDir string, signer *urlconv.Signer, version string) *Resolver {
	return &Resolver{
		app:     app,
		rootDir: rootDir,
		signer:  signer,
		version: version,
	}
}

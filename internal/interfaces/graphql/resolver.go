//go:generate go tool github.com/99designs/gqlgen

package graphql

import (
	"main/internal/application"
	"main/internal/infrastructure/urlconv"
)

type Resolver struct {
	App     *application.Root
	RootDir string
	Signer  *urlconv.Signer
	Version string
}

func NewResolver(app *application.Root, rootDir string, signer *urlconv.Signer, version string) *Resolver {
	return &Resolver{
		App:     app,
		RootDir: rootDir,
		Signer:  signer,
		Version: version,
	}
}

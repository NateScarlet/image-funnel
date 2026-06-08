//go:generate go tool github.com/99designs/gqlgen

package graphql

import (
	"main/internal/application"
	"main/internal/infrastructure/urlconv"
	"time"
)

type Resolver struct {
	app             *application.Root
	rootDir         string
	signer          *urlconv.Signer
	version         string
	baseURL         string
	serverStartTime time.Time
}

func NewResolver(app *application.Root, rootDir string, signer *urlconv.Signer, version string, baseURL string) *Resolver {
	return &Resolver{
		app:             app,
		rootDir:         rootDir,
		signer:          signer,
		version:         version,
		baseURL:         baseURL,
		serverStartTime: time.Now(),
	}
}

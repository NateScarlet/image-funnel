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
	// loadLatencyThresholdMs 加载耗时采样阈值；≤0 表示关闭，对客户端暴露为 null
	loadLatencyThresholdMs int
}

func NewResolver(app *application.Root, rootDir string, signer *urlconv.Signer, version string, baseURL string, loadLatencyThresholdMs int) *Resolver {
	return &Resolver{
		app:                    app,
		rootDir:                rootDir,
		signer:                 signer,
		version:                version,
		baseURL:                baseURL,
		serverStartTime:        time.Now(),
		loadLatencyThresholdMs: loadLatencyThresholdMs,
	}
}

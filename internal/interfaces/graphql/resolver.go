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
	// sentryDSN 下发给客户端的标准 DSN；空串表示关闭上报
	sentryDSN string
	// loadLatencyThresholdMs 加载耗时采样阈值；≤0 表示关闭，对客户端暴露为 null
	loadLatencyThresholdMs int
}

func NewResolver(app *application.Root, rootDir string, signer *urlconv.Signer, version string, baseURL string, sentryDSN string, loadLatencyThresholdMs int) *Resolver {
	return &Resolver{
		app:                    app,
		rootDir:                rootDir,
		signer:                 signer,
		version:                version,
		baseURL:                baseURL,
		serverStartTime:        time.Now(),
		sentryDSN:              sentryDSN,
		loadLatencyThresholdMs: loadLatencyThresholdMs,
	}
}

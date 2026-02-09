package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"main/internal/apperror"
	"main/internal/application"
	appdirectory "main/internal/application/directory"
	appimage "main/internal/application/image"
	appsession "main/internal/application/session"
	domdirectory "main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/session"
	"main/internal/infrastructure/concurrency"
	"main/internal/infrastructure/ebus"
	"main/internal/infrastructure/inmem"
	"main/internal/infrastructure/localfs"
	"main/internal/infrastructure/magick"
	"main/internal/infrastructure/stdimage"
	"main/internal/infrastructure/urlconv"
	"main/internal/infrastructure/xmpsidecar"
	"main/internal/interfaces/graphql"
	interfacehttp "main/internal/interfaces/http"
	"main/internal/pubsub"
	"main/internal/shared"

	gql "github.com/99designs/gqlgen/graphql"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/NateScarlet/gqlgen-batching/pkg/batching"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const defaultPort = "34898"

var version = "dev"

func main() {
	var (
		logger *zap.Logger
		err    error
	)

	logger, err = initLogger(version)
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	cfg, err := loadConfig(logger, version)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	signer := urlconv.NewSigner(cfg.SecretKey, cfg.AbsRootDir)

	sessionRepo := inmem.NewSessionRepository()
	metadataRepo := xmpsidecar.NewRepository()

	// Initialize Image Cache and Processor
	cacheDir := filepath.Join(os.TempDir(), "image-funnel-cache")
	// Cleanup every 1 hour, remove files older than 24 hours
	imageCache, cleanupCache := localfs.NewImageCache(cacheDir, time.Hour, 24*time.Hour)
	defer cleanupCache()
	magickProcessor := magick.NewProcessor(imageCache, cfg.MagickConcurrency)
	hybridProcessor := stdimage.NewHybridProcessor(magickProcessor)
	imageProcessor := concurrency.NewSingleFlightImageProcessor(hybridProcessor)

	imageFactory := image.NewFactory(metadataRepo, imageProcessor)
	dirRepo := inmem.NewDirectoryRepository(cfg.AbsRootDir)
	dirScanner := localfs.NewScanner(cfg.AbsRootDir, imageFactory, dirRepo)

	sessionTopic, _ := pubsub.NewInMemoryTopic[*session.Session]()
	fileChangedTopic, _ := pubsub.NewInMemoryTopic[*shared.FileChangedEvent]()
	eventBus := ebus.NewEventBus(sessionTopic, fileChangedTopic, appsession.NewSessionDTOFactory(signer))

	fileWatcher := localfs.NewWatcher(logger)
	_, dirServiceCleanup := domdirectory.NewService(fileWatcher, eventBus, cfg.AbsRootDir, dirRepo, logger)
	defer dirServiceCleanup()

	sessionService, sessionCleanup := session.NewService(sessionRepo, metadataRepo, dirScanner, eventBus, logger, sessionTopic, cfg.AbsRootDir)
	defer sessionCleanup()

	imageDTOFactory := appimage.NewImageDTOFactory(signer)

	sessionHandler := appsession.NewHandler(sessionService, eventBus, signer, logger)
	directoryHandler := appdirectory.NewHandler(dirScanner, eventBus, imageDTOFactory, dirRepo)

	appRoot := application.NewRoot(sessionHandler, directoryHandler)

	resolver := graphql.NewResolver(appRoot, cfg.AbsRootDir, signer, version)

	srv := handler.New(graphql.NewExecutableSchema(graphql.Config{Resolvers: resolver}))

	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Actual check is done in cors middleware
			},
		},
	})

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(batching.POST{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})
	srv.SetRecoverFunc(func(ctx context.Context, e any) error {
		logger.Error(
			"internal server error",
			zap.Any("error", e),
			zap.String("path", gql.GetPath(ctx).String()),
			zap.Stack("stack"),
		)
		return apperror.New(
			"INTERNAL_SERVER_ERROR",
			"internal server error",
			"服务器内部错误",
		)
	})
	srv.SetErrorPresenter(graphql.ErrorPresenter)
	srv.AroundFields(func(ctx context.Context, next gql.Resolver) (res interface{}, err error) {
		res, err = next(ctx)
		for i := range apperror.ExpandJoinError(err) {
			gql.AddError(ctx, i)
		}
		return res, nil
	})

	gui := playground.Handler("GraphQL Playground", "/graphql")

	httpServer := interfacehttp.NewServer(
		logger,
		signer,
		imageProcessor,
		srv,
		gui,
		cfg.AbsRootDir,
		cfg.FrontendDir,
		cfg.CorsHosts,
	)

	logger.Fatal("start server", zap.Error(httpServer.Serve(":"+cfg.Port)))
}

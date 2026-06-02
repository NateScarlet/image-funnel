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
	appmemo "main/internal/application/memo"
	appsession "main/internal/application/session"
	domdirectory "main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/memo"
	"main/internal/domain/session"
	"main/internal/infrastructure/concurrency"
	"main/internal/infrastructure/ebus"
	"main/internal/infrastructure/inmem"
	"main/internal/infrastructure/localfs"
	"main/internal/infrastructure/magick"
	"main/internal/infrastructure/stdimage"
	"main/internal/infrastructure/urlconv"
	"main/internal/infrastructure/winsleep"
	"main/internal/infrastructure/xmpsidecar"
	"main/internal/interfaces/graphql"
	interfacehttp "main/internal/interfaces/http"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"

	gql "github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"

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
	imageCache, cleanupCache := localfs.NewImageCache(cacheDir, time.Hour, 24*time.Hour, logger)
	defer cleanupCache()
	magickProcessor := magick.NewProcessor(imageCache, cfg.MagickConcurrency)
	hybridProcessor := stdimage.NewHybridProcessor(magickProcessor)
	retryProcessor := appimage.NewRetryProcessor(hybridProcessor, logger)
	imageProcessor := concurrency.NewSingleFlightImageProcessor(retryProcessor)

	imageFactory := image.NewFactory(metadataRepo, imageProcessor, cfg.AbsRootDir)
	dirRepo := inmem.NewDirectoryRepository(cfg.AbsRootDir)

	imageRepo := localfs.NewImageRepository(cfg.AbsRootDir, imageFactory, dirRepo)
	imageFilterBuilder := image.NewFilterBuilder()
	imgMover := localfs.NewImageMover(cfg.AbsRootDir, imageRepo, imageFilterBuilder)
	dirAnalyzerImpl := localfs.NewDirectoryAnalyzer(cfg.AbsRootDir, imageFactory, dirRepo)
	singleFlightDirAnalyzer := concurrency.NewSingleFlightDirectoryAnalyzer(dirAnalyzerImpl)

	imageDTOFactory := appimage.NewDTOFactory(signer, cfg.AbsRootDir)
	sessionDTOFactory := appsession.NewDTOFactory()

	sessionTopic, _ := pubsub.NewInMemoryTopic[scalar.ID](pubsub.InMemoryTopicWithCapacity(4096))
	fileChangedTopic, _ := pubsub.NewInMemoryTopic[*shared.FileChangedEvent](pubsub.InMemoryTopicWithCapacity(65536))
	eventBus := ebus.NewEventBus(sessionTopic, fileChangedTopic, sessionRepo, sessionDTOFactory)

	var dirAnalyzer domdirectory.Analyzer = singleFlightDirAnalyzer
	var statsCache *inmem.DirectoryStatsCache
	if cfg.EnableDirectoryStatsCache {
		var cache = inmem.NewDirectoryStatsCache(singleFlightDirAnalyzer, logger)
		statsCache = cache
		dirAnalyzer = cache
	}

	fileWatcher := localfs.NewWatcher(logger)
	dirSvc, dirServiceCleanup := domdirectory.NewService(fileWatcher, eventBus, cfg.AbsRootDir, dirRepo, logger)
	defer dirServiceCleanup()

	if cfg.EnableDirectoryStatsCache && statsCache != nil {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			for event, err := range fileChangedTopic.Subscribe(ctx) {
				if err != nil {
					continue
				}
				if ctx.Err() != nil {
					return
				}

				dir, err := dirSvc.GetDirectory(ctx, event.DirectoryID)
				if err != nil {
					logger.Error("failed to get directory", zap.Error(err))
					continue
				}

				statsCache.Invalidate(dir.RelPath())
			}
		}()
		defer cancel()
	}

	sessionService, sessionCleanup := session.NewService(sessionRepo, metadataRepo, imageRepo, eventBus, dirSvc, logger, sessionTopic, cfg.AbsRootDir, imageFilterBuilder)
	defer sessionCleanup()

	// 系统处理 GraphQL mutation (用户的写入交互行为) 起，在设定的闲置时间内阻止系统休眠，避免在其他设备使用时本机休眠断开连接
	sleepGuard, stopSleepGuard := winsleep.NewGuard(cfg.IdleThreshold, logger)
	defer stopSleepGuard()

	imageService := image.NewService(metadataRepo, imageRepo, cfg.AbsRootDir)

	directoryDTOFactory := appdirectory.NewDTOFactory(imageDTOFactory)
	filterBuilder := domdirectory.NewFilterBuilder()

	sessionHandler := appsession.NewHandler(sessionService, eventBus, sessionDTOFactory, imageDTOFactory, logger)
	memoDTOFactory := appmemo.NewDTOFactory(cfg.AbsRootDir)
	memoFilterBuilder := memo.NewFilterBuilder()
	directoryHandler := appdirectory.NewHandler(
		dirAnalyzer,
		eventBus,
		directoryDTOFactory,
		filterBuilder,
		dirRepo,
		dirSvc,
	)
	memoRepository := localfs.NewMemoRepository(cfg.AbsRootDir)
	memoHandler := appmemo.NewHandler(memoRepository, memo.NewService(memoRepository, cfg.AbsRootDir), dirSvc, eventBus, memoDTOFactory, memoFilterBuilder)
	imageHandler := appimage.NewHandler(imageService, eventBus, imageRepo, imgMover, dirSvc, imageDTOFactory, imageFilterBuilder, logger, cfg.AbsRootDir, imageFactory)

	appRoot := application.NewRoot(sessionHandler, directoryHandler, memoHandler, imageHandler)

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
	sleepGuard.RecordActivity() // 启动服务本身也视为一个活动
	srv.AroundOperations(func(ctx context.Context, next gql.OperationHandler) gql.ResponseHandler {
		oc := gql.GetOperationContext(ctx)
		if oc != nil && oc.Operation != nil && oc.Operation.Operation == ast.Mutation {
			sleepGuard.RecordActivity()
		}
		return next(ctx)
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

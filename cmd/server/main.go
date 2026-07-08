package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"main/internal/apperror"
	"main/internal/application"
	appdevice "main/internal/application/device"
	appdirectory "main/internal/application/directory"
	apphook "main/internal/application/hook"
	appimage "main/internal/application/image"
	appnote "main/internal/application/note"
	apppairing "main/internal/application/pairing"
	appsession "main/internal/application/session"
	ddevice "main/internal/domain/device"
	domdirectory "main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/note"
	"main/internal/domain/pairing"
	"main/internal/domain/session"
	"main/internal/infrastructure"
	"main/internal/infrastructure/clipboard"
	"main/internal/infrastructure/concurrency"
	infrahook "main/internal/infrastructure/hook"
	"main/internal/infrastructure/inmem"
	"main/internal/infrastructure/jwt"
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
	"main/internal/tokenrw"

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
	dirRepo := localfs.NewDirectoryRepository(cfg.AbsRootDir)

	var imageRepo image.Repository = localfs.NewImageRepository(cfg.AbsRootDir, imageFactory, dirRepo)
	imageFilterBuilder := image.NewFilterBuilder()
	imgMover := localfs.NewImageMover(cfg.AbsRootDir, imageRepo, imageFilterBuilder, cfg.UseSystemRecycleBin)
	dirAnalyzerImpl := localfs.NewDirectoryAnalyzer(cfg.AbsRootDir, imageFactory, dirRepo)
	singleFlightDirAnalyzer := concurrency.NewSingleFlightDirectoryAnalyzer(dirAnalyzerImpl)

	imageDTOFactory := appimage.NewDTOFactory(signer, cfg.AbsRootDir)
	sessionDTOFactory := appsession.NewDTOFactory()

	deviceDTOFactory := appdevice.NewDTOFactory()

	sessionTopic, _ := pubsub.NewInMemoryTopic[scalar.ID](pubsub.InMemoryTopicWithCapacity(4096))
	fileChangedTopic, _ := pubsub.NewInMemoryTopic[*shared.FileChangedEvent](pubsub.InMemoryTopicWithCapacity(65536))
	prCreatedTopic, _ := pubsub.NewInMemoryTopic[*pairing.Request](pubsub.InMemoryTopicWithCapacity(1024))
	prResolvedTopic, _ := pubsub.NewInMemoryTopic[*pairing.RequestResolvedEvent](pubsub.InMemoryTopicWithCapacity(1024))
	deviceSavedTopic, _ := pubsub.NewInMemoryTopic[*ddevice.Device]()
	deviceDeletedTopic, _ := pubsub.NewInMemoryTopic[scalar.ID]()
	metadataUpdatedTopic, _ := pubsub.NewInMemoryTopic[*shared.MetadataUpdatedEvent]()
	sessionCommittedTopic, _ := pubsub.NewInMemoryTopic[*shared.SessionCommittedEvent](pubsub.InMemoryTopicWithCapacity(1024))

	imageRepo = infrastructure.NewEventPublishingImageRepository(imageRepo, fileChangedTopic, dirRepo)

	revocationRepo, err := localfs.NewRevocationRepository(cfg.DataDir)
	if err != nil {
		logger.Fatal("failed to create revocation repository", zap.Error(err))
	}
	revocationList, err := localfs.NewCachedRevocationList(context.Background(), revocationRepo)
	if err != nil {
		logger.Fatal("failed to create cached revocation list", zap.Error(err))
	}
	tokenSource := jwt.NewTokenSource(10*time.Minute, 30*24*time.Hour, []byte(cfg.SecretKey))

	var dirAnalyzer domdirectory.Analyzer = singleFlightDirAnalyzer
	var statsCache *inmem.DirectoryStatsCache
	if cfg.EnableDirectoryStatsCache {
		var cache = inmem.NewDirectoryStatsCache(singleFlightDirAnalyzer, logger)
		statsCache = cache
		dirAnalyzer = cache
	}

	rawFileWatcher := localfs.NewWatcher(logger)
	fileWatcher := inmem.NewDebouncedWatcher(rawFileWatcher, 300*time.Millisecond)
	dirSvc, dirServiceCleanup := domdirectory.NewService(fileWatcher, fileChangedTopic, cfg.AbsRootDir, dirRepo, logger)
	defer dirServiceCleanup()

	// 初始化外部钩子服务
	hookRunner := infrahook.NewRunner(cfg.AbsRootDir, cfg.HooksDir, logger, metadataUpdatedTopic, fileChangedTopic, cfg.BaseURL+"/graphql", tokenSource, imageRepo, dirSvc, dirRepo)
	defer hookRunner.Close()

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

	sessionService, sessionCleanup := session.NewService(sessionRepo, metadataRepo, imageRepo, fileChangedTopic, metadataUpdatedTopic, dirSvc, logger, sessionTopic, cfg.AbsRootDir, imageFilterBuilder, hookRunner)
	defer sessionCleanup()

	// 系统处理 GraphQL mutation (用户的写入交互行为) 起，在设定的闲置时间内阻止系统休眠，避免在其他设备使用时本机休眠断开连接
	sleepGuard, stopSleepGuard := winsleep.NewGuard(cfg.IdleThreshold, logger)
	defer stopSleepGuard()

	imageService := image.NewService(metadataRepo, imageRepo, cfg.AbsRootDir, metadataUpdatedTopic)

	directoryDTOFactory := appdirectory.NewDTOFactory(imageDTOFactory)
	filterBuilder := domdirectory.NewFilterBuilder()

	directoryHandler := appdirectory.NewHandler(
		dirAnalyzer,
		directoryDTOFactory,
		filterBuilder,
		dirRepo,
		dirSvc,
		fileChangedTopic,
	)

	sessionHandler := appsession.NewHandler(
		sessionService,
		sessionDTOFactory,
		imageDTOFactory,
		logger,
		dirSvc,
		sessionTopic,
		sessionCommittedTopic,
	)
	noteDTOFactory := appnote.NewDTOFactory(cfg.AbsRootDir)
	noteFilterBuilder := note.NewFilterBuilder()
	noteRepository := localfs.NewNoteRepository(cfg.AbsRootDir)
	noteHandler := appnote.NewHandler(noteRepository, note.NewService(noteRepository, note.NewFactory(cfg.AbsRootDir)), dirSvc, fileChangedTopic, noteDTOFactory, noteFilterBuilder)
	clipboard := clipboard.NewClipboard()
	imageHandler := appimage.NewHandler(imageService, fileChangedTopic, imageRepo, imgMover, imgMover, dirSvc, imageDTOFactory, imageFilterBuilder, logger, cfg.AbsRootDir, imageFactory, clipboard)

	rawAuthRepo, err := localfs.NewDeviceRepository(cfg.DataDir)
	if err != nil {
		logger.Fatal("failed to create auth repository", zap.Error(err))
	}
	authRepo, err := inmem.NewDeviceRepository(context.Background(), rawAuthRepo)
	if err != nil {
		logger.Fatal("failed to create cached device repository", zap.Error(err))
	}
	pairingRepo := inmem.NewPairingRequestRepository()
	pairingService := pairing.NewService(pairingRepo, prCreatedTopic, prResolvedTopic, prCreatedTopic, prResolvedTopic)
	deviceFactory := ddevice.NewFactory()
	authService, err := ddevice.NewService(authRepo, pairingService, logger, cfg.WebAuthnRPID, cfg.WebAuthnRPOrigins, deviceSavedTopic, deviceDeletedTopic, revocationList, deviceFactory)
	if err != nil {
		logger.Fatal("Failed to initialize auth service", zap.Error(err))
	}
	deviceHandler := appdevice.NewHandler(authService, tokenSource, deviceDTOFactory, logger, deviceSavedTopic, deviceDeletedTopic)
	pairingDTOFactory := apppairing.NewDTOFactory()
	pairingHandler := apppairing.NewHandler(authService, pairingService, pairingDTOFactory)

	hookHandler := apphook.NewHandler(hookRunner, hookRunner, &imageServiceWrapper{svc: imageService, rootDir: cfg.AbsRootDir}, apphook.NewDTOFactory())

	appRoot := application.NewRoot(sessionHandler, directoryHandler, noteHandler, imageHandler, deviceHandler, pairingHandler, hookHandler)

	resolver := graphql.NewResolver(appRoot, cfg.AbsRootDir, signer, version, cfg.BaseURL)

	// 首次启动时自动拉起浏览器
	go func() {
		count, err := authService.Count(context.Background())
		if err != nil {
			logger.Error("failed to check user count for first-launch browser", zap.Error(err))
			return
		}
		if count == 0 {
			setupToken := authService.SetupToken()
			if setupToken != "" {
				url := fmt.Sprintf("%s/auth?setup_token=%s", cfg.BaseURL, setupToken)
				logger.Info("First launch detected, opening browser for registration", zap.String("url", url))
				// 简单的各平台打开浏览器命令
				var cmd *exec.Cmd
				switch runtime.GOOS {
				case "windows":
					cmd = exec.Command("cmd", "/c", "start", url)
				case "darwin":
					cmd = exec.Command("open", url)
				default:
					cmd = exec.Command("xdg-open", url)
				}
				cmd.Start()
			}
		}
	}()

	srv := handler.New(graphql.NewExecutableSchema(graphql.Config{
		Resolvers: resolver,
		Directives: graphql.DirectiveRoot{
			Public: func(ctx context.Context, obj any, next gql.Resolver) (res any, err error) {
				return next(ctx)
			},
		},
	}))

	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Actual check is done in cors middleware
			},
		},
		InitFunc: func(ctx context.Context, initPayload transport.InitPayload) (context.Context, *transport.InitPayload, error) {
			if authHeader, ok := initPayload["Authorization"].(string); ok {
				if tokenStr, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
					if deviceID, err := deviceHandler.ValidateToken(ctx, tokenStr); err == nil {
						ctx = appdevice.WithTrustedDevice(ctx, deviceID)
					}
				}

			}
			return ctx, nil, nil
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
		fc := gql.GetFieldContext(ctx)
		if fc != nil && fc.Field.Definition != nil {
			if fc.Object == "Mutation" || fc.Object == "Query" || fc.Object == "Subscription" {
				isPublic := false
				for _, d := range fc.Field.Definition.Directives {
					if d.Name == "public" {
						isPublic = true
						break
					}
				}
				if !isPublic && !appdevice.IsTrustedDevice(ctx) && !appdevice.IsTrustedIP(ctx) {
					err := apperror.New("UNAUTHORIZED", "unauthorized access", "未授权访问")
					gql.AddError(ctx, err)
					return nil, err
				}
			}
		}

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

	var graphqlHandler http.Handler = srv
	graphqlHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		authHeader := r.Header.Get("Authorization")
		if tokenStr, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
			if deviceID, err := deviceHandler.ValidateToken(ctx, tokenStr); err == nil {
				ctx = appdevice.WithTrustedDevice(ctx, deviceID)
			}
		}
		ctx = appdevice.WithUserAgent(ctx, r.Header.Get("User-Agent"))
		srv.ServeHTTP(w, r.WithContext(ctx))
	})
	graphqlHandler = tokenrw.CookiesMiddleware(graphqlHandler)

	httpServer := interfacehttp.NewServer(
		logger,
		signer,
		imageProcessor,
		graphqlHandler,
		gui,
		cfg.AbsRootDir,
		cfg.FrontendDir,
		cfg.CorsHosts,
		cfg.TrustedIPs,
		cfg.TrustedProxies,
	)

	logger.Fatal("start server", zap.Error(httpServer.Serve(":"+cfg.Port)))
}

type imageServiceWrapper struct {
	svc     *image.Service
	rootDir string
}

func (w *imageServiceWrapper) GetPaths(ctx context.Context, ids []string) ([]string, error) {
	var paths []string
	for _, idStr := range ids {
		id, err := scalar.ParseID(idStr)
		if err != nil {
			return nil, err
		}
		img, err := w.svc.GetImage(ctx, id)
		if err != nil {
			return nil, err
		}
		if img != nil {
			paths = append(paths, filepath.Join(w.rootDir, img.RelPath()))
		}
	}
	return paths, nil
}

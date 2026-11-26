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
	"runtime/debug"
	"strings"
	"time"

	"main/internal/apperror"
	"main/internal/application"
	appdevice "main/internal/application/device"
	appdirectory "main/internal/application/directory"
	apphook "main/internal/application/hook"
	appimage "main/internal/application/image"
	appnote "main/internal/application/note"
	appnotification "main/internal/application/notification"
	apppairing "main/internal/application/pairing"
	appsession "main/internal/application/session"
	ddevice "main/internal/domain/device"
	domdirectory "main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/note"
	domnotification "main/internal/domain/notification"
	"main/internal/domain/pairing"
	"main/internal/domain/sentry"
	"main/internal/domain/session"
	"main/internal/infrastructure"
	"main/internal/infrastructure/clipboard"
	"main/internal/infrastructure/concurrency"
	"main/internal/infrastructure/ffmpeg"
	infrahook "main/internal/infrastructure/hook"
	"main/internal/infrastructure/inmem"
	"main/internal/infrastructure/jwt"
	"main/internal/infrastructure/localfs"
	"main/internal/infrastructure/magick"
	"main/internal/infrastructure/samplearchive"
	infrasentry "main/internal/infrastructure/sentry"
	"main/internal/infrastructure/sqlite"
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
	"go.uber.org/zap/zapcore"
)

const defaultPort = "34898"

var version = "dev"

// newTrashSweeper 依据配置挑选清扫 adapter：
// 开启系统回收站时使用构建标签对应的平台 adapter，否则直接物理删除。
// 平台不支持系统回收站时由 adapter 在实际清扫时返回错误，不影响服务启动。
func newTrashSweeper(useSystemRecycleBin bool) localfs.TrashSweeper {
	if useSystemRecycleBin {
		return localfs.NewSystemTrashSweeper()
	}
	return localfs.NewPhysicalDeleteSweeper()
}

// sampleArchiveRoot 把训练样本归档根目录固定在 DataDir 下的 training-samples-v2 子目录，并解析为绝对路径。
// v2 与旧 training-samples 隔离，旧样本不读写、不迁移。
// 与加载延迟归因一致，服务端不依赖进程当前工作目录。
func sampleArchiveRoot(dataDir string) (string, error) {
	return filepath.Abs(filepath.Join(dataDir, "training-samples-v2"))
}

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

	// 上报链路与接收端共用不带钩子的 logger：任何落盘或发送失败只写日志，不回灌为新事件。
	// DSN 为空时 NewReporter 返回空实现，不会创建客户端或产生任何事件。
	safeLogger := logger
	reporter, err := infrasentry.NewReporter(cfg.SentryDSN, version, &http.Client{})
	if err != nil {
		logger.Fatal("failed to initialize sentry reporter", zap.Error(err))
	}
	logger = logger.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return infrasentry.NewReportingCore(core, reporter, safeLogger)
	}))

	signer := urlconv.NewSigner(cfg.SecretKey, cfg.AbsRootDir)

	// 自托管模式才装配 envelope 接收端点；其它模式不注册路由，本地不接收也不落盘
	var envelopeHandler http.Handler
	var envelopePath string
	if cfg.SentryMode == sentry.ModeSelf {
		store, err := localfs.NewSentryStore(filepath.Join(cfg.DataDir, "sentry"))
		if err != nil {
			logger.Fatal("failed to resolve sentry directory", zap.Error(err))
		}
		logger.Info("sentry envelope directory", zap.String("path", store.Dir()))
		envelopeHandler = interfacehttp.NewSentryEnvelopeHandler(safeLogger, store)
		envelopePath = sentry.EnvelopePath
	}

	sessionRepo := inmem.NewSessionRepository()
	metadataRepo := xmpsidecar.NewRepository()

	// Initialize Image Cache and Processor
	cacheDir := filepath.Join(os.TempDir(), "image-funnel-cache")
	// Cleanup every 1 hour, remove files older than 24 hours
	imageCache, cleanupCache := localfs.NewImageCache(cacheDir, time.Hour, 24*time.Hour, logger)
	defer cleanupCache()
	transcodeQueue := inmem.NewTranscodeQueue()
	magickProcessor := magick.NewProcessor(cfg.MagickConcurrency)
	// AVIF 编码器探测：auto 且探测可用时用 ffmpeg(SVT-AV1)，否则显式用 ImageMagick（无 nil 依赖）。
	// 编码器标识参与变体 key 计算，切换编码器不会误用另一编码器的缓存产物。
	// IMAGE_FUNNEL_IMAGE_PROCESSOR=magick 时跳过探测，所有转码强制走 ImageMagick
	avifProcessor := appimage.Processor(magickProcessor)
	avifEncoderID := appimage.EncoderMagick
	if cfg.ImageProcessor == ImageProcessorMagick {
		logger.Info("image processor forced to imagemagick by config")
	} else if avifEncoder := ffmpeg.Detect(logger); avifEncoder != nil {
		avifProcessor = avifEncoder
		avifEncoderID = avifEncoder.ID()
	}
	hybridProcessor := stdimage.NewHybridProcessor(magickProcessor, avifProcessor)
	retryProcessor := appimage.NewRetryProcessor(hybridProcessor, logger)
	// 探针与 seam 共用同一个处理器作为元数据后备来源：探针由处理器构造，
	// seam 再注入探针，避免「seam → 探针 → seam」的构造环
	sourceProbe := appimage.NewSourceProbe(retryProcessor)
	imageProcessor := appimage.NewTranscodeCoordinator(imageCache, retryProcessor, transcodeQueue, sourceProbe, avifEncoderID, logger)
	transcodeCtx, stopTranscodeWorkers := context.WithCancel(context.Background())
	defer stopTranscodeWorkers()
	imageProcessor.StartWorkers(transcodeCtx, int(cfg.MagickConcurrency))

	imageFactory := image.NewFactory(metadataRepo, imageProcessor, cfg.AbsRootDir)
	dirRepo := localfs.NewDirectoryRepository(cfg.AbsRootDir)
	dirRenamer := localfs.NewDirectoryRenamer(cfg.AbsRootDir)

	var imageRepo image.Repository = localfs.NewImageRepository(cfg.AbsRootDir, imageFactory, dirRepo)
	imageFilterBuilder := image.NewFilterBuilder()
	imgMover := localfs.NewImageMover(cfg.AbsRootDir, imageRepo, imageFilterBuilder)
	imgTrasher := localfs.NewImageTrasher(cfg.AbsRootDir, imageRepo, imageFilterBuilder, newTrashSweeper(cfg.UseSystemRecycleBin), logger)
	dirAnalyzerImpl := localfs.NewDirectoryAnalyzer(cfg.AbsRootDir, imageFactory, dirRepo)
	singleFlightDirAnalyzer := concurrency.NewSingleFlightDirectoryAnalyzer(dirAnalyzerImpl)

	imageDTOFactory := appimage.NewDTOFactory(signer, cfg.AbsRootDir)
	sessionDTOFactory := appsession.NewDTOFactory()

	deviceDTOFactory := appdevice.NewDTOFactory()

	sessionTopic, _ := pubsub.NewInMemoryTopic[scalar.ID](pubsub.InMemoryTopicWithCapacity(4096))
	fileChangedTopic, _ := pubsub.NewInMemoryTopic[*domdirectory.FileChangedEvent](pubsub.InMemoryTopicWithCapacity(65536))
	// 笔记保存完成事件在服务端全部处理（含钩子回写）结束后发布，供 noteSaved 订阅消费
	noteSavedTopic, _ := pubsub.NewInMemoryTopic[*note.SavedEvent](pubsub.InMemoryTopicWithCapacity(4096))
	savedEvents := appnote.NewSavedEvents(noteSavedTopic, appnote.DefaultSavedEventTimeout, logger)
	prCreatedTopic, _ := pubsub.NewInMemoryTopic[*pairing.Request](pubsub.InMemoryTopicWithCapacity(1024))
	prResolvedTopic, _ := pubsub.NewInMemoryTopic[*pairing.RequestResolvedEvent](pubsub.InMemoryTopicWithCapacity(1024))
	deviceSavedTopic, _ := pubsub.NewInMemoryTopic[*ddevice.Device]()
	deviceDeletedTopic, _ := pubsub.NewInMemoryTopic[scalar.ID]()
	metadataUpdatedTopic, _ := pubsub.NewInMemoryTopic[*shared.MetadataUpdatedEvent]()
	sessionCommittedTopic, _ := pubsub.NewInMemoryTopic[*shared.SessionCommittedEvent](pubsub.InMemoryTopicWithCapacity(1024))
	notifChangedTopic, _ := pubsub.NewInMemoryTopic[*shared.NotificationChangedEventDTO](pubsub.InMemoryTopicWithCapacity(4096))

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
	dirSvc, dirServiceCleanup := domdirectory.NewService(fileWatcher, fileChangedTopic, cfg.AbsRootDir, dirRepo, dirRenamer, logger)
	defer dirServiceCleanup()

	notifFilterBuilder := domnotification.NewFilterBuilder()
	notifDBPath := filepath.Join(cfg.DataDir, "notifications.db")
	sqliteNotifRepo, notifCleanup, err := sqlite.NewNotificationRepository(notifDBPath, notifFilterBuilder,
		sqlite.WithReclaimErrorHandler(func(err error) {
			logger.Error("notification reclaim error", zap.Error(err))
		}),
	)
	if err != nil {
		logger.Fatal("failed to initialize sqlite notification repository", zap.Error(err))
	}
	defer notifCleanup()

	notifDTOFactory := appnotification.NewDTOFactory()
	notifFactory := &domnotification.Factory{}
	notifService := domnotification.NewService(sqliteNotifRepo, notifFactory, notifChangedTopic)
	notifHandler := appnotification.NewHandler(sqliteNotifRepo, notifService, notifDTOFactory, notifFilterBuilder, notifChangedTopic, safeLogger)

	// 初始化外部钩子服务： Hook 脚本在本地执行，直接连接本机的监听端口
	localGraphQLURL := fmt.Sprintf("http://127.0.0.1:%s/graphql", cfg.Port)
	hookRunner := infrahook.NewRunner(infrahook.Deps{
		RootDir:            cfg.AbsRootDir,
		HooksDir:           cfg.HooksDir,
		DataDir:            cfg.DataDir,
		GraphQLURL:         localGraphQLURL,
		Logger:             logger,
		MetadataUpdatedSub: metadataUpdatedTopic,
		FileChangedSub:     fileChangedTopic,
		TokenSource:        tokenSource,
		ImageRepo:          imageRepo,
		DirRepo:            dirRepo,
		NotificationSender: notifHandler,
		SavedEvents:        savedEvents,
	})
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

	// 提交后流水线的 ctx 由服务端持有：随服务关闭取消，后台任务不复用已结束的请求 ctx
	pipelineCtx, cancelPipeline := context.WithCancel(context.Background())
	var commitPipeline session.CommitPipeline
	var waitPipeline func()
	if cfg.EnableTrainingSampleArchive {
		// 训练样本归档开启时落盘到 <DataDir>/training-samples-v2/，布局为 <DataDir>/training-samples-v2/cas/<分片>/<h>.data
		dataRoot, err := sampleArchiveRoot(cfg.DataDir)
		if err != nil {
			logger.Fatal("failed to resolve training sample data directory", zap.Error(err))
		}
		// 采样与钩子共用一个点名 logger：设置 IMAGE_FUNNEL_DEBUG_LOGGERS=commit-pipeline
		// 即可通过通用机制只打开这条流水线的 Debug 耗时日志，不影响其它 logger。
		pipelineLogger := logger.Named(commitPipelineLoggerName)
		// 解码缓存与差异区域并集只在单次 Commit 内有效：在最外层为每次 Commit 新建
		// 解码器与并集，并注入到生成器/记录器，Commit 结束即丢弃，避免跨 Commit 泄漏。
		recorderFactory := session.SampleRecorderFactoryFunc(func() (session.SampleRecorder, error) {
			decoder := samplearchive.NewCommitImageDecoder(cfg.AbsRootDir)
			union := samplearchive.NewRegionUnion()
			generator := samplearchive.NewInProcessSampleGenerator(dataRoot, decoder, union, pipelineLogger)
			return samplearchive.NewRecorder(dataRoot, cfg.AbsRootDir, generator, samplearchive.PNGPromptReader{}, pipelineLogger)
		})
		asyncPipeline := session.NewAsyncCommitPipeline(pipelineCtx, recorderFactory, hookRunner, dirSvc, pipelineLogger)
		commitPipeline = asyncPipeline
		waitPipeline = asyncPipeline.Wait
		logger.Info("training sample archive enabled", zap.String("path", dataRoot))
	} else {
		commitPipeline = session.NewNullCommitPipeline(pipelineCtx, session.NullSampleRecorder{}, hookRunner, dirSvc, logger)
	}
	defer func() {
		cancelPipeline()
		if waitPipeline != nil {
			waitPipeline()
		}
	}()

	sessionService, sessionCleanup := session.NewService(sessionRepo, metadataRepo, imageRepo, fileChangedTopic, metadataUpdatedTopic, dirSvc, logger, sessionTopic, cfg.AbsRootDir, imageFilterBuilder, commitPipeline)
	defer sessionCleanup()

	// 系统处理 GraphQL mutation (用户的写入交互行为) 起，在设定的闲置时间内阻止系统休眠，避免在其他设备使用时本机休眠断开连接
	sleepGuard, stopSleepGuard := winsleep.NewGuard(cfg.IdleThreshold, logger)
	defer stopSleepGuard()

	imageService := image.NewService(metadataRepo, imageRepo, cfg.AbsRootDir, metadataUpdatedTopic)

	directoryDTOFactory := appdirectory.NewDTOFactory(imageDTOFactory)
	filterBuilder := domdirectory.NewFilterBuilder()

	directoryHandler := appdirectory.NewHandler(
		safeLogger,
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
		safeLogger,
		dirSvc,
		sessionTopic,
		sessionCommittedTopic,
	)
	noteDTOFactory := appnote.NewDTOFactory(cfg.AbsRootDir)
	noteFilterBuilder := note.NewFilterBuilder()
	noteRepository := localfs.NewNoteRepository(cfg.AbsRootDir)
	noteHandler := appnote.NewHandler(safeLogger, noteRepository, note.NewService(noteRepository, note.NewFactory(cfg.AbsRootDir), cfg.AbsRootDir), dirSvc, fileChangedTopic, noteDTOFactory, noteFilterBuilder, savedEvents)
	clipboard := clipboard.NewClipboard()
	imageHandler := appimage.NewHandler(imageService, fileChangedTopic, imageRepo, imgMover, imgTrasher, dirSvc, imageDTOFactory, imageFilterBuilder, safeLogger, cfg.AbsRootDir, imageFactory, clipboard, hookRunner)

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
	deviceHandler := appdevice.NewHandler(authService, tokenSource, deviceDTOFactory, safeLogger, deviceSavedTopic, deviceDeletedTopic)
	pairingDTOFactory := apppairing.NewDTOFactory()
	pairingHandler := apppairing.NewHandler(safeLogger, authService, pairingService, pairingDTOFactory)

	hookHandler := apphook.NewHandler(safeLogger, hookRunner, hookRunner, &imageServiceWrapper{svc: imageService, rootDir: cfg.AbsRootDir}, apphook.NewDTOFactory())

	appRoot := application.NewRoot(sessionHandler, directoryHandler, noteHandler, imageHandler, deviceHandler, pairingHandler, hookHandler, notifHandler)

	resolver := graphql.NewResolver(appRoot, cfg.AbsRootDir, signer, version, cfg.BaseURL, cfg.SentryDSN, cfg.LoadLatencyThresholdMs)

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
		// panic 显式上报：用 "panic" 字段名避免日志 hook 把同一事件重复上报。
		logger.Error(
			"internal server error",
			zap.Any("panic", e),
			zap.String("path", gql.GetPath(ctx).String()),
			zap.Stack("stack"),
		)
		if err := reporter.Report(ctx, sentry.Event{
			Panic:       e,
			Stack:       debug.Stack(),
			Fingerprint: []string{"server", "panic"},
		}); err != nil {
			safeLogger.Error("failed to report panic", zap.Error(err))
		}
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
			// 应用层用安全 logger 避免重复上报；resolver 的意外错误在此统一进入日志 hook
			if sentry.ShouldReport(i) {
				logger.Error("resolver failed", zap.Error(i), zap.String("path", gql.GetPath(ctx).String()))
			}
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
		sourceProbe,
		graphqlHandler,
		gui,
		cfg.AbsRootDir,
		cfg.FrontendDir,
		cfg.CorsHosts,
		cfg.TrustedIPs,
		cfg.TrustedProxies,
		envelopeHandler,
		envelopePath,
		cfg.IsDev,
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
			paths = append(paths, img.RelPath().Abs(w.rootDir))
		}
	}
	return paths, nil
}

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	"main/internal/pubsub"
	"main/internal/shared"

	gql "github.com/99designs/gqlgen/graphql"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/NateScarlet/gqlgen-batching/pkg/batching"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

const defaultPort = "34898"

var version = "dev"

func mustGenerateRandomSecretKey() string {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func main() {
	var logger *zap.Logger
	var err error

	if version != "dev" {
		logger, err = zap.NewProduction()
	} else {
		logger, err = zap.NewDevelopment()
	}
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	port := os.Getenv("IMAGE_FUNNEL_PORT")
	if port == "" {
		port = defaultPort
	}

	rootDir := os.Getenv("IMAGE_FUNNEL_ROOT_DIR")
	if rootDir == "" {
		rootDir = "."
	}

	absRootDir, err := filepath.Abs(rootDir)
	if err != nil {
		log.Fatalf("Failed to resolve root directory: %v", err)
	}

	secretKey := os.Getenv("IMAGE_FUNNEL_SECRET_KEY")
	if secretKey == "" {
		secretKey = mustGenerateRandomSecretKey()
		logger.Info("generated random secret key for this session")
	}

	signer := urlconv.NewSigner(secretKey, absRootDir)

	sessionRepo := inmem.NewSessionRepository()
	metadataRepo := xmpsidecar.NewRepository()

	// Initialize Image Cache and Processor
	cacheDir := filepath.Join(os.TempDir(), "image-funnel-cache")
	// Cleanup every 1 hour, remove files older than 24 hours
	imageCache, cleanupCache := localfs.NewImageCache(cacheDir, time.Hour, 24*time.Hour)
	defer cleanupCache()
	magickProcessor := magick.NewProcessor(imageCache)
	hybridProcessor := stdimage.NewHybridProcessor(magickProcessor)
	imageProcessor := concurrency.NewSingleFlightImageProcessor(hybridProcessor)

	imageFactory := image.NewFactory(metadataRepo, imageProcessor)
	dirRepo := inmem.NewDirectoryRepository(absRootDir)
	dirScanner := localfs.NewScanner(absRootDir, imageFactory, dirRepo)

	sessionTopic, _ := pubsub.NewInMemoryTopic[*session.Session]()
	fileChangedTopic, _ := pubsub.NewInMemoryTopic[*shared.FileChangedEvent]()
	eventBus := ebus.NewEventBus(sessionTopic, fileChangedTopic, appsession.NewSessionDTOFactory(signer))

	fileWatcher := localfs.NewWatcher(logger)
	_, dirServiceCleanup := domdirectory.NewService(fileWatcher, eventBus, absRootDir, dirRepo, logger)
	defer dirServiceCleanup()

	// Watch root dir
	// Service will start watching rootDir automatically

	sessionService, sessionCleanup := session.NewService(sessionRepo, metadataRepo, dirScanner, eventBus, logger, sessionTopic)
	defer sessionCleanup()

	imageDTOFactory := appimage.NewImageDTOFactory(signer)

	sessionHandler := appsession.NewHandler(sessionService, eventBus, signer, logger)
	directoryHandler := appdirectory.NewHandler(dirScanner, eventBus, imageDTOFactory, dirRepo)

	appRoot := application.NewRoot(sessionHandler, directoryHandler)

	resolver := graphql.NewResolver(appRoot, absRootDir, signer, version)

	srv := handler.New(graphql.NewExecutableSchema(graphql.Config{Resolvers: resolver}))

	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
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

	var frontendDir string
	isProduction := version != "dev"

	execPath, err := os.Executable()
	if err != nil {
		logger.Warn("get executable path", zap.Error(err))
		execPath = "."
	}
	execDir := filepath.Dir(execPath)

	if isProduction {
		frontendDir = filepath.Join(execDir, "dist")
	} else {
		frontendDir = filepath.Join("frontend", "dist")
	}

	if _, err := os.Stat(frontendDir); os.IsNotExist(err) {
		logger.Warn("frontend directory not found", zap.String("path", frontendDir))
	}

	r := mux.NewRouter()

	r.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			f := negotiateFormat(r, "application/json", "text/html")
			if f == "text/html" {
				gui.ServeHTTP(w, r)
				return
			}
		}
		srv.ServeHTTP(w, r)
	})

	r.HandleFunc("/image", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		relativePath := query.Get("path")
		timestamp := query.Get("t")
		size := query.Get("s")
		signature := query.Get("sig")
		widthStr := query.Get("w")
		qualityStr := query.Get("q")
		raw := query.Has("raw")

		if relativePath == "" || timestamp == "" || size == "" || signature == "" {
			http.Error(w, "missing required parameters", http.StatusBadRequest)
			return
		}

		err := signer.ValidateRequestFromValues(query)
		if err != nil {
			http.Error(w, "invalid signature: "+err.Error(), http.StatusForbidden)
			return
		}

		absPath := filepath.Join(absRootDir, relativePath)
		if raw {
			http.ServeFile(w, r, absPath)
			return
		}

		width := 0
		if widthStr != "" {
			if w, err := strconv.Atoi(widthStr); err == nil {
				width = w
			}
		}

		quality := 0
		if qualityStr != "" {
			if q, err := strconv.Atoi(qualityStr); err == nil {
				quality = q
			}
		}

		processedPath, err := imageProcessor.Process(r.Context(), absPath, width, quality)
		if errors.Is(err, context.Canceled) {
			http.Error(w, "request canceled", http.StatusRequestTimeout)
			return
		}
		if err != nil {
			logger.Error("process image", zap.Error(err))
			http.ServeFile(w, r, absPath)
			return
		}

		http.ServeFile(w, r, processedPath)
	})

	addStaticRoutes(r, frontendDir)

	logger.Info("starting server",
		zap.String("port", port),
		zap.String("rootDir", absRootDir),
		zap.String("version", version),
		zap.String("frontendDir", frontendDir),
	)
	logger.Fatal("start server", zap.Error(http.ListenAndServe(":"+port, r)))
}

func negotiateFormat(r *http.Request, formats ...string) string {
	accept := r.Header.Get("Accept")
	if accept == "" && len(formats) > 0 {
		return formats[0]
	}

	for _, format := range formats {
		if strings.Contains(accept, format) {
			return format
		}
	}

	if len(formats) > 0 {
		return formats[0]
	}

	return ""
}

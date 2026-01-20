package main

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"main/internal/application"
	"main/internal/application/directory"
	appimage "main/internal/application/image"
	appsession "main/internal/application/session"
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

func generateRandomSecretKey() string {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		log.Printf("Warning: Failed to generate random secret key, using fallback")
		return "fallback-secret-key-change-in-production"
	}
	return base64.StdEncoding.EncodeToString(key)
}

func main() {
	logger, err := zap.NewProduction()
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
		secretKey = generateRandomSecretKey()
		logger.Info("Generated random secret key for this session")
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

	dirScanner := localfs.NewScanner(absRootDir, metadataRepo, imageProcessor)
	sessionService := session.NewService(sessionRepo, metadataRepo, dirScanner)
	sessionTopic, _ := pubsub.NewInMemoryTopic[*shared.SessionDTO]()
	eventBus := ebus.NewEventBus(sessionTopic)

	imageDTOFactory := appimage.NewImageDTOFactory(signer)
	sessionHandler := appsession.NewHandler(sessionService, eventBus, signer, logger)
	directoryHandler := directory.NewHandler(dirScanner, imageDTOFactory)

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

	gui := playground.Handler("GraphQL Playground", "/graphql")

	var frontendDir string
	isProduction := version != "dev"

	execPath, err := os.Executable()
	if err != nil {
		log.Printf("Warning: Failed to get executable path: %v", err)
		execPath = "."
	}
	execDir := filepath.Dir(execPath)

	if isProduction {
		frontendDir = filepath.Join(execDir, "dist")
		log.Printf("Running in production mode (version: %s), serving frontend from: %s", version, frontendDir)
	} else {
		frontendDir = filepath.Join("frontend", "dist")
		log.Printf("Running in development mode (version: %s), serving frontend from: %s", version, frontendDir)
	}

	if _, err := os.Stat(frontendDir); os.IsNotExist(err) {
		log.Printf("Warning: Frontend directory not found at %s", frontendDir)
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
		if err != nil {
			log.Printf("Image processing failed: %v", err)
			http.ServeFile(w, r, absPath)
			return
		}

		http.ServeFile(w, r, processedPath)
	})

	addStaticRoutes(r, frontendDir)

	log.Printf("🚀 Server ready at http://localhost:%s", port)
	log.Printf("📁 Root directory: %s", absRootDir)
	log.Printf("🏷️  Version: %s", version)
	log.Fatal(http.ListenAndServe(":"+port, r))
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

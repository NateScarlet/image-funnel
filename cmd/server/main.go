package main

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"main/internal/application"
	"main/internal/application/directory"
	appsession "main/internal/application/session"
	"main/internal/domain/session"
	"main/internal/infrastructure/ebus"
	"main/internal/infrastructure/inmem"
	"main/internal/infrastructure/localfs"
	"main/internal/infrastructure/urlconv"
	"main/internal/infrastructure/xmpsidecar"
	"main/internal/interfaces/graphql"
	"main/internal/pubsub"
	"main/internal/shared"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gorilla/mux"
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
		log.Printf("Generated random secret key for this session")
	}

	signer := urlconv.NewSigner(secretKey, absRootDir)

	sessionRepo := inmem.NewSessionRepository()
	metadataRepo := xmpsidecar.NewRepository()
	dirScanner := localfs.NewScanner(absRootDir, metadataRepo)
	sessionService := session.NewService(sessionRepo, metadataRepo, dirScanner)
	sessionTopic, _ := pubsub.NewInMemoryTopic[*shared.SessionDTO]()
	eventBus := ebus.NewEventBus(sessionTopic)

	sessionHandler := appsession.NewHandler(sessionService, eventBus, signer)
	directoryHandler := directory.NewHandler(dirScanner)

	appRoot := application.NewRoot(sessionHandler, directoryHandler)

	resolver := graphql.NewResolver(appRoot, absRootDir, signer, version)

	srv := handler.NewDefaultServer(graphql.NewExecutableSchema(graphql.Config{Resolvers: resolver}))
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
		relativePath := r.URL.Query().Get("path")
		timestamp := r.URL.Query().Get("t")
		signature := r.URL.Query().Get("sig")

		if relativePath == "" || timestamp == "" || signature == "" {
			http.Error(w, "missing required parameters", http.StatusBadRequest)
			return
		}

		absPath := filepath.Join(absRootDir, relativePath)

		valid, err := signer.ValidateRequest(relativePath, timestamp, signature)
		if err != nil || !valid {
			http.Error(w, "invalid signature", http.StatusForbidden)
			return
		}

		if !strings.HasPrefix(absPath, absRootDir) {
			http.Error(w, "invalid path", http.StatusForbidden)
			return
		}

		if !xmpsidecar.IsSupportedImage(absPath) {
			http.Error(w, "unsupported image type", http.StatusBadRequest)
			return
		}

		http.ServeFile(w, r, absPath)
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

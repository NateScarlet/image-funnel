package main

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"main/graph"
	"main/internal/url"
	"main/internal/xmp"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

const defaultPort = "8000"

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

	signer := url.NewSigner(secretKey, absRootDir)
	resolver := graph.NewResolver(absRootDir, signer)

	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{Resolvers: resolver}))
	gui := playground.Handler("GraphQL Playground", "/graphql")

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:5173", "http://localhost:8080"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})

	// Determine frontend directory based on environment
	var frontendDir string
	env := os.Getenv("IMAGE_FUNNEL_ENV")

	// Get the directory of the executable itself
	execPath, err := os.Executable()
	if err != nil {
		log.Printf("Warning: Failed to get executable path: %v", err)
		execPath = "."
	}
	execDir := filepath.Dir(execPath)

	if env == "production" {
		// Production: use dist directory relative to executable
		frontendDir = filepath.Join(execDir, "dist")
		log.Printf("Running in production mode, serving frontend from: %s", frontendDir)
	} else {
		// Development: use frontend/dist directory relative to project root
		// For development, we still use project root relative path for consistency
		frontendDir = filepath.Join("frontend", "dist")
		log.Printf("Running in development mode, serving frontend from: %s", frontendDir)
	}

	if _, err := os.Stat(frontendDir); os.IsNotExist(err) {
		log.Printf("Warning: Frontend directory not found at %s", frontendDir)
	}

	r := mux.NewRouter()
	r.Use(c.Handler)

	// Handle GraphQL endpoint with playground
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

		if !xmp.IsSupportedImage(absPath) {
			http.Error(w, "unsupported image type", http.StatusBadRequest)
			return
		}

		http.ServeFile(w, r, absPath)
	})

	// Add static routes
	addStaticRoutes(r, frontendDir)

	log.Printf("🚀 Server ready at http://localhost:%s", port)
	log.Printf("📁 Root directory: %s", absRootDir)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

// negotiateFormat mimics gin's NegotiateFormat for content negotiation
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

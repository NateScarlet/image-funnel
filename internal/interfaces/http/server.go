package http

import (
	"net/http"
	"strings"

	"main/internal/infrastructure/urlconv"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"go.uber.org/zap"
)

type Server struct {
	logger         *zap.Logger
	signer         *urlconv.Signer
	imageProcessor ImageProcessor
	graphqlHandler http.Handler
	playground     http.Handler
	absRootDir     string
	frontendDir    string
	corsHosts      []string
}

func NewServer(
	logger *zap.Logger,
	signer *urlconv.Signer,
	imageProcessor ImageProcessor,
	graphqlHandler http.Handler,
	playground http.Handler,
	absRootDir string,
	frontendDir string,
	corsHosts []string,
) *Server {
	return &Server{
		logger:         logger,
		signer:         signer,
		imageProcessor: imageProcessor,
		graphqlHandler: graphqlHandler,
		playground:     playground,
		absRootDir:     absRootDir,
		frontendDir:    frontendDir,
		corsHosts:      corsHosts,
	}
}

func (s *Server) Serve(addr string) error {
	r := mux.NewRouter()

	r.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			f := s.negotiateFormat(r, "application/json", "text/html")
			if f == "text/html" {
				s.playground.ServeHTTP(w, r)
				return
			}
		}
		s.graphqlHandler.ServeHTTP(w, r)
	})

	r.HandleFunc("/image", handleImage(s.logger, s.signer, s.imageProcessor, s.absRootDir))

	addStaticRoutes(r, s.frontendDir)

	handler := cors.New(cors.Options{
		AllowOriginFunc: func(origin string) bool {
			return isOriginAllowed(origin, "", s.corsHosts)
		},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "X-Apollo-Tracing", "Apollo-Query-Plan"},
		AllowCredentials: true,
	}).Handler(r)

	s.logger.Info("starting server",
		zap.String("addr", addr),
		zap.String("rootDir", s.absRootDir),
		zap.String("frontendDir", s.frontendDir),
	)

	return http.ListenAndServe(addr, handler)
}

func (s *Server) negotiateFormat(r *http.Request, formats ...string) string {
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

package http

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	appdevice "main/internal/application/device"
	appimage "main/internal/application/image"
	"main/internal/infrastructure/urlconv"
	"main/internal/tokenrw"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"go.uber.org/zap"
)

type Server struct {
	logger         *zap.Logger
	signer         *urlconv.Signer
	imageProcessor appimage.Processor
	graphqlHandler http.Handler
	playground     http.Handler
	absRootDir     string
	frontendDir    string
	corsHosts      []string
	trustedIPs     []netip.Prefix
	trustedProxies []netip.Prefix
}

func NewServer(
	logger *zap.Logger,
	signer *urlconv.Signer,
	imageProcessor appimage.Processor,
	graphqlHandler http.Handler,
	playground http.Handler,
	absRootDir string,
	frontendDir string,
	corsHosts []string,
	trustedIPs []netip.Prefix,
	trustedProxies []netip.Prefix,
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
		trustedIPs:     trustedIPs,
		trustedProxies: trustedProxies,
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
		AllowedHeaders:   []string{"Content-Type", "Authorization", "X-Apollo-Tracing", "Apollo-Query-Plan", "Token-Transfer"},
		AllowCredentials: true,
	}).Handler(r)

	// Real IP middleware
	next := handler
	handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		remoteIPStr := req.RemoteAddr
		if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			remoteIPStr = host
		}

		remoteIP, err := netip.ParseAddr(remoteIPStr)
		if err == nil {
			isTrustedProxy := false
			for _, prefix := range s.trustedProxies {
				if prefix.Contains(remoteIP) {
					isTrustedProxy = true
					break
				}
			}

			if isTrustedProxy {
				if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
					ips := strings.Split(xff, ",")
					if len(ips) > 0 {
						realIP := strings.TrimSpace(ips[0])
						req.RemoteAddr = realIP
					}
				} else if xri := req.Header.Get("X-Real-IP"); xri != "" {
					req.RemoteAddr = strings.TrimSpace(xri)
				}

				// Re-parse after overriding
				if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
					remoteIPStr = host
				} else {
					remoteIPStr = req.RemoteAddr
				}
				remoteIP, err = netip.ParseAddr(remoteIPStr)
			}

			if err == nil {
				isTrustedIP := false
				for _, prefix := range s.trustedIPs {
					if prefix.Contains(remoteIP) {
						isTrustedIP = true
						break
					}
				}
				ctx := req.Context()
				ctx = appdevice.WithTrustedIP(ctx, isTrustedIP)
				ctx = appdevice.WithRemoteIP(ctx, remoteIPStr)
				req = req.WithContext(ctx)
			}
		}

		next.ServeHTTP(w, req)
	})

	s.logger.Info("starting server",
		zap.String("addr", addr),
		zap.String("rootDir", s.absRootDir),
		zap.String("frontendDir", s.frontendDir),
	)

	handler = tokenrw.TransferMiddleware(handler)

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

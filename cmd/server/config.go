package main

import (
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

type Config struct {
	Port                      string
	RootDir                   string
	AbsRootDir                string
	SecretKey                 string
	CorsHosts                 []string
	IsDev                     bool
	FrontendDir               string
	MagickConcurrency         int64
	EnableDirectoryStatsCache bool
	IdleThreshold             time.Duration
	TrustedIPs                []netip.Prefix
	TrustedProxies            []netip.Prefix
	DataDir                   string
	WebAuthnRPID              string
	WebAuthnRPOrigins         []string
	BaseURL                   string
	HooksDir                  string
	UseSystemRecycleBin       bool
}

func loadConfig(logger *zap.Logger, version string) (*Config, error) {
	isDev := version == "dev"

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
		return nil, err
	}

	secretKey := os.Getenv("IMAGE_FUNNEL_SECRET_KEY")
	if secretKey == "" {
		secretKey = mustGenerateRandomSecretKey()
		logger.Info("generated random secret key for this session")
	}

	corsHosts := []string{}
	if v := os.Getenv("IMAGE_FUNNEL_CORS_HOSTS"); v != "" {
		corsHosts = strings.Split(v, ",")
	}

	execPath, err := os.Executable()
	if err != nil {
		logger.Warn("get executable path", zap.Error(err))
		execPath = "."
	}
	execDir := filepath.Dir(execPath)

	var frontendDir string
	if !isDev {
		frontendDir = filepath.Join(execDir, "dist")
	} else {
		frontendDir = filepath.Join("frontend", "dist")
	}

	if _, err := os.Stat(frontendDir); os.IsNotExist(err) {
		logger.Warn("frontend directory not found", zap.String("path", frontendDir))
	}

	magickConcurrency := int64(4)
	if v := os.Getenv("IMAGE_FUNNEL_MAGICK_CONCURRENCY"); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			magickConcurrency = i
		} else {
			logger.Warn("invalid IMAGE_FUNNEL_MAGICK_CONCURRENCY, use default", zap.String("value", v))
		}
	}

	enableDirectoryStatsCache := true
	if v := os.Getenv("IMAGE_FUNNEL_ENABLE_DIRECTORY_STATS_CACHE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			enableDirectoryStatsCache = b
		} else {
			logger.Warn("invalid IMAGE_FUNNEL_ENABLE_DIRECTORY_STATS_CACHE, use default", zap.String("value", v))
		}
	}

	idleThreshold := 5 * time.Minute
	if v := os.Getenv("IMAGE_FUNNEL_IDLE_THRESHOLD"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			idleThreshold = d
		} else {
			logger.Warn("invalid IMAGE_FUNNEL_IDLE_THRESHOLD, use default", zap.String("value", v))
		}
	}

	var trustedIPs []netip.Prefix
	trustedIPStr := os.Getenv("IMAGE_FUNNEL_TRUSTED_IP")
	if trustedIPStr == "" {
		trustedIPs = append(trustedIPs, netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128"))
	} else {
		for _, v := range strings.Split(trustedIPStr, ",") {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if !strings.Contains(v, "/") {
				// Assume single IP, try to parse and add /32 or /128
				if addr, err := netip.ParseAddr(v); err == nil {
					trustedIPs = append(trustedIPs, netip.PrefixFrom(addr, addr.BitLen()))
					continue
				}
			}
			if prefix, err := netip.ParsePrefix(v); err == nil {
				trustedIPs = append(trustedIPs, prefix)
			} else {
				logger.Warn("invalid IMAGE_FUNNEL_TRUSTED_IP segment", zap.String("value", v), zap.Error(err))
			}
		}
	}

	dataDir := os.Getenv("IMAGE_FUNNEL_DATA_DIR")
	if dataDir == "" {
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			logger.Warn("failed to get UserConfigDir, fallback to current dir", zap.Error(err))
			userConfigDir = "."
		}
		dataDir = filepath.Join(userConfigDir, "io.github.natescarlet.image-funnel")
	}

	baseURL := os.Getenv("IMAGE_FUNNEL_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:" + port
	}

	webauthnRPID := os.Getenv("IMAGE_FUNNEL_WEBAUTHN_RPID")
	if webauthnRPID == "" {
		if baseURL != "" {
			if u, err := url.Parse(baseURL); err == nil {
				webauthnRPID = u.Hostname()
			}
		}
		if webauthnRPID == "" {
			webauthnRPID = "localhost"
		}
	}
	webauthnRPOriginsStr := os.Getenv("IMAGE_FUNNEL_WEBAUTHN_RP_ORIGINS")
	var webauthnRPOrigins []string
	if webauthnRPOriginsStr != "" {
		webauthnRPOrigins = strings.Split(webauthnRPOriginsStr, ",")
	} else {
		// Default to BaseURL, and if it's localhost, add 127.0.0.1 for convenience
		webauthnRPOrigins = []string{baseURL}
		if u, err := url.Parse(baseURL); err == nil && u.Hostname() == "localhost" {
			u.Host = strings.Replace(u.Host, "localhost", "127.0.0.1", 1)
			webauthnRPOrigins = append(webauthnRPOrigins, u.String())
		}
	}
	trustedProxiesStr := os.Getenv("IMAGE_FUNNEL_TRUSTED_PROXY")
	var trustedProxies []netip.Prefix
	if trustedProxiesStr == "" {
		trustedProxies = []netip.Prefix{
			netip.MustParsePrefix("127.0.0.0/8"),
			netip.MustParsePrefix("::1/128"),
		}
	} else {
		for _, p := range strings.Split(trustedProxiesStr, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if !strings.Contains(p, "/") {
				if strings.Contains(p, ":") {
					p += "/128"
				} else {
					p += "/32"
				}
			}
			if prefix, err := netip.ParsePrefix(p); err == nil {
				trustedProxies = append(trustedProxies, prefix)
			} else {
				logger.Warn("invalid IMAGE_FUNNEL_TRUSTED_PROXY segment", zap.String("value", p), zap.Error(err))
			}
		}
	}

	hooksDir := os.Getenv("IMAGE_FUNNEL_HOOK_DIR")

	useSystemRecycleBin := false
	if v := os.Getenv("IMAGE_FUNNEL_USE_SYSTEM_RECYCLE_BIN"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			useSystemRecycleBin = b
		} else {
			logger.Warn("invalid IMAGE_FUNNEL_USE_SYSTEM_RECYCLE_BIN, use default", zap.String("value", v))
		}
	}

	return &Config{
		Port:                      port,
		RootDir:                   rootDir,
		AbsRootDir:                absRootDir,
		SecretKey:                 secretKey,
		CorsHosts:                 corsHosts,
		IsDev:                     isDev,
		FrontendDir:               frontendDir,
		MagickConcurrency:         magickConcurrency,
		EnableDirectoryStatsCache: enableDirectoryStatsCache,
		IdleThreshold:             idleThreshold,
		TrustedIPs:                trustedIPs,
		TrustedProxies:            trustedProxies,
		DataDir:                   dataDir,
		WebAuthnRPID:              webauthnRPID,
		WebAuthnRPOrigins:         webauthnRPOrigins,
		BaseURL:                   baseURL,
		HooksDir:                  hooksDir,
		UseSystemRecycleBin:       useSystemRecycleBin,
	}, nil
}

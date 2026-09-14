package main

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ImageProcessor 选择图片转码用的处理器。
// 零值（""）等同 ImageProcessorAuto：调用方只需判断是否为 ImageProcessorMagick
type ImageProcessor string

const (
	// ImageProcessorAuto 启动时探测 ffmpeg AVIF 编码器，可用则使用，否则回退 ImageMagick
	ImageProcessorAuto ImageProcessor = "auto"
	// ImageProcessorMagick 跳过探测，所有转码强制走 ImageMagick
	ImageProcessorMagick ImageProcessor = "magick"
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
	ImageProcessor            ImageProcessor
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

	// 图片处理器选择：auto 时优先 ffmpeg AVIF 编码（快且画质不低于基准），
	// 探测失败自动回退 ImageMagick；magick 时跳过探测，所有转码强制走 ImageMagick。
	// 显式写错的值直接报错，避免用户以为已生效而实际仍走 ffmpeg
	var imageProcessor ImageProcessor
	switch v := os.Getenv("IMAGE_FUNNEL_IMAGE_PROCESSOR"); v {
	case "":
		imageProcessor = ImageProcessorAuto
	case string(ImageProcessorAuto), string(ImageProcessorMagick):
		imageProcessor = ImageProcessor(v)
	default:
		return nil, fmt.Errorf("invalid IMAGE_FUNNEL_IMAGE_PROCESSOR %q: must be %q or %q",
			v, ImageProcessorAuto, ImageProcessorMagick)
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
		ImageProcessor:            imageProcessor,
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

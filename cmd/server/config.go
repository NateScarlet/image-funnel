package main

import (
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

type Config struct {
	Port        string
	RootDir     string
	AbsRootDir  string
	SecretKey   string
	CorsHosts   []string
	IsDev       bool
	FrontendDir string
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

	return &Config{
		Port:        port,
		RootDir:     rootDir,
		AbsRootDir:  absRootDir,
		SecretKey:   secretKey,
		CorsHosts:   corsHosts,
		IsDev:       isDev,
		FrontendDir: frontendDir,
	}, nil
}

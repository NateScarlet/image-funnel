package hook

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"go.uber.org/zap"
)

// configSource 钩子配置来源。
// 每次调用都重新读盘解析，保留「改 toml 即时生效」的语义：不做缓存，也不引入热重载。
type configSource interface {
	Load() ([]hookConfig, error)
}

// tomlConfigSource 从 hooksDir 逐个读取 *.toml 的配置来源实现
type tomlConfigSource struct {
	hooksDir string
	logger   *zap.Logger
}

func newTOMLConfigSource(hooksDir string, logger *zap.Logger) *tomlConfigSource {
	return &tomlConfigSource{hooksDir: hooksDir, logger: logger}
}

var _ configSource = (*tomlConfigSource)(nil)

func (s *tomlConfigSource) Load() ([]hookConfig, error) {
	if s.hooksDir == "" {
		return nil, nil
	}
	var configs []hookConfig

	entries, err := os.ReadDir(s.hooksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if entry.IsDir() || ext != ".toml" {
			continue
		}

		path := filepath.Join(s.hooksDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			s.logger.Warn("failed to read hook config file", zap.String("path", path), zap.Error(err))
			continue
		}

		var cfg hookConfig
		dec := toml.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			s.logger.Warn("failed to parse hook config toml", zap.String("path", path), zap.Error(err))
			continue
		}

		if cfg.Directive != nil {
			if cfg.Directive.OnSuccessAction == "" {
				cfg.Directive.OnSuccessAction = "COMMENT_OUT"
			}
			if cfg.Directive.OnFailAction == "" {
				cfg.Directive.OnFailAction = "COMMENT_OUT"
			}
		}

		cfg.Filename = entry.Name()
		if cfg.ID == "" {
			cfg.ID = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		if cfg.Command == "" {
			s.logger.Warn("hook command is empty, skip loading", zap.String("id", cfg.ID))
			continue
		}

		configs = append(configs, cfg)
	}

	return configs, nil
}

package hook

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"

	domhook "main/internal/domain/hook"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/pelletier/go-toml/v2"
	"go.uber.org/zap"
)

// imageDispatchTrigger 图片手动触发分发器定义
type imageDispatchTrigger struct {
}

// copyTrigger 复制增强能力标记：声明 [copy] 即让脚本在用户复制图片时被同步调用，
// 由脚本自行读取图片元数据并返回应写入剪贴板的内容（请求期同步内容提供者，非事件触发器）
type copyTrigger struct {
}

// autocompleteConfig 自动完成脚本配置
type autocompleteConfig struct {
	Command  string `toml:"command"`
	Protocol string `toml:"protocol"` // "json-rpc" 启用常驻复用；缺省（空）为单次执行
}

// directiveConfig 钩子提供的笔记指令配置
type directiveConfig struct {
	Name            string              `toml:"name"`
	Usage           string              `toml:"usage"`
	OnSuccessAction string              `toml:"on_success_action"`
	OnFailAction    string              `toml:"on_fail_action"`
	Autocomplete    *autocompleteConfig `toml:"autocomplete"`
}

// hookConfig 声明式 Hook 配置文件对应的 TOML 数据结构
type hookConfig struct {
	Filename    string           `toml:"-"`           // 配置文件名（如 foo.toml），不参与 TOML 解析
	ID          string           `toml:"id"`
	Name        string           `toml:"name"`
	Description string           `toml:"description"`
	Command     string           `toml:"command"`
	Order       int              `toml:"order"` // 执行顺序，默认 0，升序排列
	Directive   *directiveConfig `toml:"directive"`
	Copy        *copyTrigger     `toml:"copy"` // 复制增强能力标记
	On          struct {
		PostUpdateImageMetadata *filters              `toml:"post_update_image_metadata"`
		ImageDispatch           *imageDispatchTrigger `toml:"image_dispatch"`
		PostUpdateNote          *struct {
			IgnoreDirective bool `toml:"ignore_directive"`
		} `toml:"post_update_note"`
		PostCommitSession *struct {
			NoteScan *struct {
				IgnoreDirective bool `toml:"ignore_directive"`
			} `toml:"note_scan"`
		} `toml:"post_commit_session"`
		NoteDispatch *struct {
		} `toml:"note_dispatch"`
	} `toml:"on"`
	Env map[string]string `toml:"env"` // 允许在 TOML 中配置自定义环境变量，键值对都为字符串
}

// filters 元数据更新筛选条件，专门用于外部钩子中的事件过滤
type filters struct {
	ID     []scalar.ID `toml:"id"`
	Rating []int       `toml:"rating"`
	Label  []string    `toml:"label"`
	Query  string      `toml:"query"`
}

// match 评估单个事件是否匹配当前的筛选条件
func (f *filters) match(event *shared.MetadataUpdatedEvent) bool {
	if f == nil {
		return true
	}

	if len(f.ID) > 0 && !slices.Contains(f.ID, event.ID) {
		return false
	}

	if len(f.Rating) > 0 && !slices.Contains(f.Rating, event.Rating) {
		return false
	}

	if len(f.Label) > 0 {
		matched := false
		eventLabelLower := strings.ToLower(event.Label)
		for _, l := range f.Label {
			if strings.ToLower(l) == eventLabelLower {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if f.Query != "" {
		queryLower := strings.ToLower(f.Query)
		filename := filepath.Base(event.Path)
		if !strings.Contains(strings.ToLower(filename), queryLower) {
			return false
		}
	}

	return true
}

func (r *Runner) loadHooks() ([]hookConfig, error) {
	if r.hooksDir == "" {
		return nil, nil
	}
	var configs []hookConfig

	entries, err := os.ReadDir(r.hooksDir)
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

		path := filepath.Join(r.hooksDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			r.logger.Warn("failed to read hook config file", zap.String("path", path), zap.Error(err))
			continue
		}

		var cfg hookConfig
		dec := toml.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			r.logger.Warn("failed to parse hook config toml", zap.String("path", path), zap.Error(err))
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
			r.logger.Warn("hook command is empty, skip loading", zap.String("id", cfg.ID))
			continue
		}

		configs = append(configs, cfg)
	}

	return configs, nil
}

func toDomainDirective(cfg *directiveConfig) *domhook.Directive {
	if cfg == nil {
		return nil
	}
	autocomplete := cfg.Autocomplete != nil && cfg.Autocomplete.Command != ""
	return &domhook.Directive{
		Name:            cfg.Name,
		Usage:           cfg.Usage,
		OnSuccessAction: cfg.OnSuccessAction,
		OnFailAction:    cfg.OnFailAction,
		Autocomplete:    autocomplete,
	}
}

// sortByOrderAndFilename 按 (order, Filename) 升序排序的比较函数，用于 SortStableFunc
func sortByOrderAndFilename(a, b hookConfig) int {
	if a.Order != b.Order {
		return a.Order - b.Order
	}
	return strings.Compare(a.Filename, b.Filename)
}

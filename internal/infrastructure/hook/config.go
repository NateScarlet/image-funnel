package hook

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	domhook "main/internal/domain/hook"
	"main/internal/scalar"
	"main/internal/shared"
)

// imageDispatchTriggerConfig 图片手动触发分发器定义
type imageDispatchTriggerConfig struct {
	// Hotkeys 在图片派发上下文中触发该钩子的快捷键声明，语法沿用前端的组合键表示（如 "ctrl+shift+k"，不区分大小写）。
	// 后端只做透传，是否与其它快捷键冲突由前端判定，冲突的键会降级为不可用并在快捷键列表中标注。
	Hotkeys []string `toml:"hotkeys"`
}

// copyTriggerConfig 复制增强能力标记：声明 [copy] 即让脚本在用户复制图片时被同步调用，
// 由脚本自行读取图片元数据并返回应写入剪贴板的内容（请求期同步内容提供者，非事件触发器）
type copyTriggerConfig struct {
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
	Filename    string             `toml:"-"` // 配置文件名（如 foo.toml），不参与 TOML 解析
	ID          string             `toml:"id"`
	Name        string             `toml:"name"`
	Description string             `toml:"description"`
	Command     string             `toml:"command"`
	Order       int                `toml:"order"` // 执行顺序，默认 0，升序排列
	Directive   *directiveConfig   `toml:"directive"`
	Copy        *copyTriggerConfig `toml:"copy"` // 复制增强能力标记
	On          struct {
		PostUpdateImageMetadata *filters                    `toml:"post_update_image_metadata"`
		ImageDispatch           *imageDispatchTriggerConfig `toml:"image_dispatch"`
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

// domainID 构造钩子对应的领域实体并取出其 ID。
// 钩子 ID 由领域实体编码，配置层不得自行解析，因此所有 ID 比对都经由此处。
func domainID(h hookConfig) scalar.ID {
	return domhook.FromRepository(h.ID, h.Name, h.Description, h.On.ImageDispatch != nil, nil, h.On.NoteDispatch != nil, nil, false, false).ID()
}

// resolveHookConfig 按领域 ID 定位钩子配置
func resolveHookConfig(hooks []hookConfig, hookID scalar.ID) (hookConfig, error) {
	for _, h := range hooks {
		if domainID(h) == hookID {
			return h, nil
		}
	}
	return hookConfig{}, fmt.Errorf("hook %s not found", hookID.String())
}

// sortByOrderAndFilename 按 (order, Filename) 升序排序的比较函数，用于 SortStableFunc
func sortByOrderAndFilename(a, b hookConfig) int {
	if a.Order != b.Order {
		return a.Order - b.Order
	}
	return strings.Compare(a.Filename, b.Filename)
}

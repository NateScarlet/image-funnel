package hook

import (
	"context"
	"strings"

	"main/internal/scalar"
)

// AutocompleteSuggestion 自动完成建议值对象
type AutocompleteSuggestion struct {
	Text        string
	DisplayText string
	Description string
	Type        string
	Style       string
}

// CopyContent 复制增强内容值对象：外部钩子脚本提供的应写入剪贴板的内容
type CopyContent struct {
	Content     string // 写入剪贴板的文本内容
	Description string // 成功通知文案，空表示脚本未提供
}

// Directive 钩子提供的笔记指令值对象
type Directive struct {
	Name            string // 指令名称，用于 slash command 匹配
	Usage           string // 使用说明，包含参数格式和选项描述
	OnSuccessAction string // 执行成功后的行为，如 COMMENT_OUT, REMOVE, KEEP
	OnFailAction    string // 执行失败后的行为，如 COMMENT_OUT, REMOVE, KEEP
	Autocomplete    bool   // 是否支持指令参数自动补全
}

// Hook 外部钩子领域实体
type Hook struct {
	id                           scalar.ID
	name                         string
	description                  string
	canDispatchByImage           bool
	canDispatchByNote            bool
	directive                    *Directive // nil 表示该 hook 不提供指令
	hasPostUpdateNote            bool
	hasPostCommitSessionNoteScan bool
}

// FromRepository 从仓库构造外部钩子领域实体，进行安全隔离 ID 编码
func FromRepository(
	rawID string,
	name, description string,
	canDispatchByImage bool,
	canDispatchByNote bool,
	directive *Directive,
	hasPostUpdateNote bool,
	hasPostCommitSessionNoteScan bool,
) *Hook {
	return &Hook{
		id:                           encodeID(rawID),
		name:                         name,
		description:                  description,
		canDispatchByImage:           canDispatchByImage,
		canDispatchByNote:            canDispatchByNote,
		directive:                    directive,
		hasPostUpdateNote:            hasPostUpdateNote,
		hasPostCommitSessionNoteScan: hasPostCommitSessionNoteScan,
	}
}

func encodeID(rawID string) scalar.ID {
	if strings.HasPrefix(rawID, "hk:") {
		return scalar.ToID(rawID)
	}
	return scalar.ToID("hk:" + rawID)
}

func decodeID(id scalar.ID) string {
	return strings.TrimPrefix(id.String(), "hk:")
}

func (h *Hook) ID() scalar.ID                      { return h.id }
func (h *Hook) Name() string                       { return h.name }
func (h *Hook) Description() string                { return h.description }
func (h *Hook) CanDispatchByImage() bool           { return h.canDispatchByImage }
func (h *Hook) CanDispatchByNote() bool            { return h.canDispatchByNote }
func (h *Hook) Directive() *Directive              { return h.directive }
func (h *Hook) HasPostUpdateNote() bool            { return h.hasPostUpdateNote }
func (h *Hook) HasPostCommitSessionNoteScan() bool { return h.hasPostCommitSessionNoteScan }

// Repository 钩子领域持久化层接口
type Repository interface {
	List(ctx context.Context) ([]*Hook, error)
}

// Runner 外部钩子执行器接口
type Runner interface {
	Trigger(ctx context.Context, ids []string, paths []string, hookID scalar.ID, triggerName string) error
	OnCommitSession(ctx context.Context, dirRelPath string) error
	TriggerForNote(ctx context.Context, noteRelPath string, hookID scalar.ID) error
	Autocomplete(ctx context.Context, hookID scalar.ID, noteRelPath string, linePrefix string, query string) ([]*AutocompleteSuggestion, error)
	CopyContent(ctx context.Context, imageID scalar.ID, imageRelPath string) (*CopyContent, error)
}

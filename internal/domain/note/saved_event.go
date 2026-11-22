package note

import (
	"context"
	"iter"

	"main/internal/domain/relpath"
)

// SavedEvent 笔记保存完成事件：服务端对笔记的全部处理（含写入与钩子回写）结束后发布。
// 事件只是信号，订阅端按相对路径读取磁盘上的最终内容。
type SavedEvent struct {
	RelPath relpath.Path
}

// SavedEvents 笔记保存完成事件的发布与订阅端口。
// Saved 登记一次应用层保存（待钩子链路终结），Finalized 在全链路处理完成后终结并发布。
type SavedEvents interface {
	// Saved 登记一次应用层保存已完成写入；若钩子链路未及时终结，超时后兜底发布事件
	Saved(ctx context.Context, relPath relpath.Path)
	// Processing 钩子链路已接手该笔记，暂停短兜底计时，避免慢脚本期间发布中间内容
	Processing(ctx context.Context, relPath relpath.Path)
	// Finalized 终结待处理状态并发布一次保存完成事件；外部编辑直接修改笔记时同样调用
	Finalized(ctx context.Context, relPath relpath.Path)
	// Subscribe 订阅保存完成事件
	Subscribe(ctx context.Context) iter.Seq2[*SavedEvent, error]
}

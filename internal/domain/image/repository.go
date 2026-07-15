package image

import (
	"context"
	"iter"
	"main/internal/shared"
	"time"
)

type Repository interface {
	Get(ctx context.Context, relPath string) (*Image, error)
	Find(ctx context.Context, relPath string) iter.Seq2[*Image, error]
}

// Mover 负责根据过滤条件物理移动图片及其伴随文件（如元数据、描述文本等）
type Mover interface {
	// Move 移动匹配过滤规则的图片到目标目录中，toDirRelPath 是相对于项目根目录的路径
	Move(ctx context.Context, relPath string, filterBy shared.ImageFilters, toDirRelPath string) (movedCount int, targetAbsDir string, err error)
}

// Trasher 负责回收站暂存、还原、清空以及历史记录的获取与管理
type Trasher interface {
	// Trash 将匹配过滤规则的图片移动到回收站目录中，并返回生成的历史ID与文件总数
	Trash(ctx context.Context, relPath string, filterBy shared.ImageFilters, message string) (historyId string, totalFileCount int, err error)
	// UndoTrash 撤销指定的回收站移动操作，将文件还原回原位
	UndoTrash(ctx context.Context, historyId string) (*shared.UndoTrashResultDTO, error)
	// EmptyTrash 手动清空回收站中早于指定保留期限的记录，投递到操作系统物理回收站
	EmptyTrash(ctx context.Context, minAge time.Duration) (clearedCount int, err error)
	// FindTrashHistory 遍历并返回当前已暂存的回收站历史列表
	FindTrashHistory(ctx context.Context) iter.Seq2[*shared.TrashHistoryItemDTO, error]
}

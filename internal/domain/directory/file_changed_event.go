package directory

import (
	"time"

	"main/internal/domain/relpath"
	"main/internal/scalar"
	"main/internal/shared"
)

// FileChangedEvent 文件变更事件：由文件系统监听转换而来的应用层事件，包含变更文件所在的目录 ID。
//
// 定义在领域层而非 shared —— shared 是无逻辑基础结构，反向依赖 domain 会让两层互相咬合；
// 事件携带根相对路径，因此必须与产生它的领域对象放在同一层。
type FileChangedEvent struct {
	DirectoryID scalar.ID // 变更文件所在的目录ID
	RelPath     relpath.Path
	Action      shared.FileAction
	OccurredAt  time.Time
}

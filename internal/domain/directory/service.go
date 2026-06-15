package directory

import (
	"context"
	"iter"
	"main/internal/apperror"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// EventBus 事件总线接口
type EventBus interface {
	PublishFileChanged(ctx context.Context, event *shared.FileChangedEvent)
}

// Service 目录领域服务
// 负责监听文件变更并转换为应用层事件
type Service struct {
	watcher  Watcher
	eventBus EventBus
	rootDir  string
	logger   *zap.Logger
	repo     Repository
}

// NewService 创建目录服务
func NewService(watcher Watcher, eventBus EventBus, rootDir string, repo Repository, logger *zap.Logger) (*Service, func()) {
	s := &Service{
		watcher:  watcher,
		eventBus: eventBus,
		rootDir:  rootDir,
		logger:   logger,
		repo:     repo,
	}

	// 启动后台监听
	ctx, cancel := context.WithCancel(context.Background())
	go s.watchAndTransform(ctx)


	cleanup := func() {
		cancel()
	}

	return s, cleanup
}

// watchAndTransform 监听文件变更并转换为事件发布
func (s *Service) watchAndTransform(ctx context.Context) {
	for fileChange, err := range s.watcher.Watch(ctx, s.rootDir) {
		if err != nil {
			s.logger.Error("file watch error", zap.Error(err))
			continue
		}

		// 将绝对路径转换为相对路径
		relPath, err := filepath.Rel(s.rootDir, fileChange.absPath)
		if err != nil {
			s.logger.Error("failed to get relative path",
				zap.String("path", fileChange.absPath),
				zap.String("root", s.rootDir),
				zap.Error(err))
			continue
		}

		// 编码目录ID
		dir, err := s.repo.Get(ctx, filepath.Dir(relPath))
		if err != nil {
			s.logger.Error("failed to get directory by path",
				zap.String("path", filepath.Dir(relPath)),
				zap.Error(err))
			continue
		}

		// 构建应用层事件
		event := &shared.FileChangedEvent{
			DirectoryID: dir.ID(),
			RelPath:     relPath,
			Action:      fileChange.action,
			OccurredAt:  fileChange.occurredAt,
		}

		// 发布事件
		s.eventBus.PublishFileChanged(ctx, event)
	}
}


// GetDirectory 根据目录 ID 获取目录实体，由领域层内部解码 ID
func (s *Service) GetDirectory(ctx context.Context, id scalar.ID) (*Directory, error) {
	relPath, err := decodeID(id)
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, relPath)
}

// ResolvePathInput 解析并校验 PathInput，返回相对于根目录的规范化相对路径。
// 若解析失败或路径逃逸根目录，则返回错误。
func (s *Service) ResolvePathInput(ctx context.Context, currentRelPath string, input shared.PathInput) (string, error) {
	var resolvedRelPath string
	var modeCount int

	if input.Absolute != "" {
		rel, err := filepath.Rel(s.rootDir, input.Absolute)
		if err != nil {
			return "", apperror.New("PATH_INVALID", "invalid absolute path", "无效的绝对路径")
		}
		resolvedRelPath = rel
		modeCount++
	}
	if input.RelativeToRoot != "" {
		resolvedRelPath = input.RelativeToRoot
		modeCount++
	}
	if input.RelativeToCurrent != "" {
		resolvedRelPath = filepath.Join(currentRelPath, input.RelativeToCurrent)
		modeCount++
	}

	if modeCount == 0 {
		return "", apperror.New("PATH_INVALID", "path input is empty", "路径输入不能为空")
	}
	if modeCount > 1 {
		return "", apperror.New("PATH_INVALID", "multiple path modes specified", "只能指定一种路径模式")
	}

	resolvedRelPath = filepath.Clean(resolvedRelPath)

	// 安全校验：确保最终目标路径在配置的根目录范围内，防止目录穿越
	if err := util.EnsurePathInRoot(s.rootDir, resolvedRelPath); err != nil {
		return "", apperror.New("PATH_INVALID", "path escapes root directory", "路径超出项目根目录范围")
	}

	return resolvedRelPath, nil
}

// SuggestDirectories 根据当前所在目录的相对路径和用户当前的路径输入，智能建议匹配的子目录迭代器
func (s *Service) SuggestDirectories(ctx context.Context, currentRelPath string, input shared.PathInput) iter.Seq2[*Directory, error] {
	return func(yield func(*Directory, error) bool) {
		var rawInput string
		var testInput shared.PathInput

		if input.Absolute != "" {
			rawInput = input.Absolute
			basePath, _ := splitPathForSuggest(rawInput)
			if basePath == "" {
				// 绝对路径若没有斜杠，则不具备合法前缀，无法提供联想，直接返回
				return
			}
			testInput.Absolute = basePath
		} else if input.RelativeToRoot != "" {
			rawInput = input.RelativeToRoot
			basePath, _ := splitPathForSuggest(rawInput)
			if basePath == "" {
				basePath = "."
			}
			testInput.RelativeToRoot = basePath
		} else {
			rawInput = input.RelativeToCurrent
			basePath, _ := splitPathForSuggest(rawInput)
			if basePath == "" {
				basePath = "."
			}
			testInput.RelativeToCurrent = basePath
		}

		_, searchTerm := splitPathForSuggest(rawInput)

		baseRelPath, err := s.ResolvePathInput(ctx, currentRelPath, testInput)
		if err != nil {
			yield(nil, err)
			return
		}

		searchTermLower := strings.ToLower(searchTerm)

		for dir, err := range s.repo.Find(ctx, baseRelPath) {
			if err != nil {
				// 当父目录路径在磁盘上不存在时，作为未找到错误优雅忽略，返回空列表而非报错
				if apperror.IsNotFound(err) {
					return
				}
				yield(nil, err)
				return
			}

			name := filepath.Base(dir.RelPath())
			// 过滤隐藏目录
			if strings.HasPrefix(name, ".") {
				continue
			}

			// 忽略大小写的子串包含匹配（同级模糊匹配）
			if searchTermLower == "" || strings.Contains(strings.ToLower(name), searchTermLower) {
				if !yield(dir, nil) {
					return
				}
			}
		}
	}
}

func splitPathForSuggest(p string) (basePath, searchTerm string) {
	i := strings.LastIndexAny(p, "/\\")
	if i < 0 {
		return "", p
	}
	return p[:i], p[i+1:]
}

// ReadState 读取指定目录的状态配置
func (s *Service) ReadState(ctx context.Context, id scalar.ID) (*shared.DirectoryStateDTO, error) {
	dir, err := s.GetDirectory(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.repo.ReadState(ctx, dir.RelPath())
}

// WriteState 写入指定目录的状态配置并更新更新时间
func (s *Service) WriteState(ctx context.Context, id scalar.ID, state *shared.DirectoryStateDTO) error {
	dir, err := s.GetDirectory(ctx, id)
	if err != nil {
		return err
	}
	if state != nil {
		state.UpdatedAt = time.Now()
	}
	return s.repo.WriteState(ctx, dir.RelPath(), state)
}

// SaveLastSession 保存上一次活跃会话的历史配置到该目录的持久化状态文件中
func (s *Service) SaveLastSession(
	ctx context.Context,
	directoryID scalar.ID,
	sessionID scalar.ID,
	filter *shared.ImageFilters,
	targetKeep int,
) error {
	dir, err := s.GetDirectory(ctx, directoryID)
	if err != nil {
		return err
	}
	state, err := s.repo.ReadState(ctx, dir.RelPath())
	if err != nil {
		return err
	}
	if state == nil {
		state = &shared.DirectoryStateDTO{}
	}
	var ratingVal []int
	var labelVal []string
	var queryVal string
	if filter != nil {
		ratingVal = filter.Rating
		labelVal = filter.Label
		queryVal = filter.Query
	}
	state.LastSession = &shared.DirectoryStateLastSessionDTO{
		ID: sessionID,
		Filter: shared.ImageFilters{
			Rating: ratingVal,
			Label:  labelVal,
			Query:  queryVal,
		},
		TargetKeep: targetKeep,
	}
	state.UpdatedAt = time.Now()
	return s.repo.WriteState(ctx, dir.RelPath(), state)
}


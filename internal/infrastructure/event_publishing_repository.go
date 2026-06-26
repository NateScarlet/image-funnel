package infrastructure

import (
	"context"
	"errors"
	"iter"
	"main/internal/apperror"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/pubsub"
	"main/internal/shared"
	"path/filepath"
	"time"
)

// EventPublishingImageRepository 图像仓库装饰器
// 负责拦截加载失败（文件不存在）并在后台补发删除事件
type EventPublishingImageRepository struct {
	repo           image.Repository
	fileChangedPub pubsub.Topic[*shared.FileChangedEvent]
	dirRepo        directory.Repository
}

// NewEventPublishingImageRepository 创建一个图像仓库装饰器
func NewEventPublishingImageRepository(repo image.Repository, fileChangedPub pubsub.Topic[*shared.FileChangedEvent], dirRepo directory.Repository) image.Repository {
	return &EventPublishingImageRepository{
		repo:           repo,
		fileChangedPub: fileChangedPub,
		dirRepo:        dirRepo,
	}
}

// Get 包装底层 Repository 的 Get 方法，并在磁盘查验文件不存在时自动补发删除事件，维护一致性状态
func (r *EventPublishingImageRepository) Get(ctx context.Context, relPath string) (*image.Image, error) {
	img, err := r.repo.Get(ctx, relPath)
	if err != nil {
		if apperror.IsNotFound(err) {
			dir, errDir := r.dirRepo.Get(ctx, filepath.Dir(relPath))
			if errDir != nil {
				// 若目录也已被删除，可直接忽略该目录获取错误并返回原文件不存在错误
				if apperror.IsNotFound(errDir) {
					return nil, err
				}
				// 其它严重的读取错误不应被静默吞掉，与原错误合并后返回
				return nil, errors.Join(err, errDir)
			}

			r.fileChangedPub.Publish(ctx, &shared.FileChangedEvent{
				DirectoryID: dir.ID(),
				RelPath:     relPath,
				Action:      shared.FileActionRemove,
				OccurredAt:  time.Now(),
			})
		}
		return nil, err
	}
	return img, nil
}

// Find 包装底层 Repository 的 Find 方法并透传
func (r *EventPublishingImageRepository) Find(ctx context.Context, relPath string) iter.Seq2[*image.Image, error] {
	return r.repo.Find(ctx, relPath)
}

package session

import (
	"context"
	"main/internal/apperror"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"os"
	"time"
)

// #region Session Methods

// UpdateLabel 更新会话中特定图片的标签值，返回更新后的图片对象
func (s *Session) UpdateLabel(imageID scalar.ID, label string) (*image.Image, error) {
	idx, ok := s.indexByID[imageID]
	if !ok {
		return nil, apperror.NewErrDocumentNotFound(imageID)
	}

	oldImg := s.images[idx]
	oldXMP := oldImg.XMPData()

	// 重新构建 XMPData 元数据，保留其它字段（rating, action, timestamp）
	var newXMP *metadata.Data
	if oldXMP != nil {
		newXMP = metadata.NewXMPData(oldXMP.Rating(), oldXMP.Action(), oldXMP.Timestamp(), label)
	} else {
		newXMP = metadata.NewXMPData(0, "", time.Time{}, label)
	}

	// 创建包含新元数据的新图片实体
	newImg := image.New(
		oldImg.ID(),
		oldImg.Filename(),
		oldImg.AbsPath(),
		s.DirectoryID(),
		oldImg.Size(),
		oldImg.ModTime(),
		newXMP,
		oldImg.Width(),
		oldImg.Height(),
	)

	// 更新内存中的图片对象，维持 images 切片和队列中引用的索引稳定
	s.images[idx] = newImg
	s.updatedAt = time.Now()

	return newImg, nil
}

// #endregion

// #region Service Methods

// UpdateLabel 更新图片的标签，并且将更改即时写入 XMP 伴随文件，然后通知相关订阅者
func (s *Service) UpdateLabel(ctx context.Context, sessionID scalar.ID, imageID scalar.ID, label string) (*image.Image, error) {
	// 获取会话锁
	sess, release, err := s.sessionRepo.Acquire(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer release()

	idx, ok := sess.indexByID[imageID]
	if !ok {
		return nil, apperror.NewErrDocumentNotFound(imageID)
	}
	img := sess.images[idx]

	// 校验修改时间以防止对过时版本的图片进行修改
	info, err := os.Stat(img.AbsPath())
	if err != nil {
		return nil, err
	}
	if info.ModTime().UnixNano() != img.ModTime().UnixNano() {
		return nil, apperror.New(
			"VERSION_CONFLICT",
			"image file has been modified on disk",
			"图片在磁盘上已被修改，操作已拒绝",
		)
	}

	// 1. 准备要持久化写入的 XMP 元数据，保留其它字段
	oldXMP := img.XMPData()
	var newXMP *metadata.Data
	if oldXMP != nil {
		newXMP = metadata.NewXMPData(oldXMP.Rating(), oldXMP.Action(), oldXMP.Timestamp(), label)
	} else {
		newXMP = metadata.NewXMPData(0, "", time.Time{}, label)
	}

	// 2. 直接持久化写入磁盘对应的 .xmp 文件，实现与会话提交解耦的即时更新
	if err := s.metadataRepo.Write(img.AbsPath(), newXMP); err != nil {
		return nil, err
	}

	// 3. 更新会话内存中的 Image 对象，使变更对当前连接的客户端即时生效
	newImg, err := sess.UpdateLabel(imageID, label)
	if err != nil {
		return nil, err
	}

	// 4. 发布会话变更事件，触发 GraphQL Subscription 通知其它客户端
	s.sessionSaved.Publish(ctx, sess.ID())

	return newImg, nil
}

// #endregion

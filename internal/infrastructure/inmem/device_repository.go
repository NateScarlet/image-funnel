package inmem

import (
	"context"
	"iter"
	"os"
	"sync"

	"main/internal/domain/device"
	"main/internal/scalar"
)

// #region DeviceRepository 带内存缓存的设备仓库

// DeviceRepository 为 device.Repository 提供基于内存的缓存装饰器，将底层存储的数据全量加载到内存中
type DeviceRepository struct {
	repo    device.Repository
	devices map[scalar.ID]*device.Device
	mu      sync.RWMutex
}

// NewDeviceRepository 创建并从底层仓储初始化一个 DeviceRepository 实例
func NewDeviceRepository(ctx context.Context, repo device.Repository) (*DeviceRepository, error) {
	c := &DeviceRepository{
		repo:    repo,
		devices: make(map[scalar.ID]*device.Device),
	}

	// 首次启动时，全量加载底层存储中的设备列表到内存缓存中
	for d, err := range repo.Find(ctx) {
		if err != nil {
			return nil, err
		}
		c.devices[d.ID()] = d
	}

	return c, nil
}

// Save 将设备保存到底层存储并同步更新内存缓存
func (r *DeviceRepository) Save(ctx context.Context, dev *device.Device) error {
	// 先持久化到磁盘
	if err := r.repo.Save(ctx, dev); err != nil {
		return err
	}

	// 更新内存缓存
	r.mu.Lock()
	r.devices[dev.ID()] = dev
	r.mu.Unlock()
	return nil
}

// Get 从内存缓存中 O(1) 获取设备，若不存在则返回 os.ErrNotExist
func (r *DeviceRepository) Get(ctx context.Context, id scalar.ID) (*device.Device, error) {
	r.mu.RLock()
	d, ok := r.devices[id]
	r.mu.RUnlock()

	if !ok {
		return nil, os.ErrNotExist
	}
	return d, nil
}

// Delete 从底层存储中删除设备并同步从内存缓存移除
func (r *DeviceRepository) Delete(ctx context.Context, id scalar.ID) error {
	// 先持久化删除
	if err := r.repo.Delete(ctx, id); err != nil {
		return err
	}

	// 更新内存缓存
	r.mu.Lock()
	delete(r.devices, id)
	r.mu.Unlock()
	return nil
}

// Find 返回内存中缓存的所有设备的迭代器，在读锁保护下拷贝快照，避免 yield 阶段的锁竞争
func (r *DeviceRepository) Find(ctx context.Context) iter.Seq2[*device.Device, error] {
	return func(yield func(*device.Device, error) bool) {
		r.mu.RLock()
		devices := make([]*device.Device, 0, len(r.devices))
		for _, d := range r.devices {
			devices = append(devices, d)
		}
		r.mu.RUnlock()

		for _, d := range devices {
			if !yield(d, nil) {
				return
			}
		}
	}
}

// 编译时接口检查
var _ device.Repository = (*DeviceRepository)(nil)

// #endregion

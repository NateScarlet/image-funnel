package localfs

import (
	"context"
	"encoding/json"
	"iter"
	"os"
	"path/filepath"
	"sync"
	"time"

	"main/internal/domain/device"
	"main/internal/util"
)

// #region RevocationRepository 文件持久化的吊销令牌存储

// RevocationRepository 负责将吊销的刷新令牌 JTI 持久化到磁盘
type RevocationRepository struct {
	dataDir string
	mu      sync.RWMutex
}

// NewRevocationRepository 创建并初始化吊销仓库
func NewRevocationRepository(dataDir string) (*RevocationRepository, error) {
	err := os.MkdirAll(dataDir, 0755)
	if err != nil {
		return nil, err
	}
	return &RevocationRepository{
		dataDir: dataDir,
	}, nil
}

func (r *RevocationRepository) filename() string {
	return filepath.Join(r.dataDir, "revocations.jsonl")
}

type revocationEntry struct {
	JTI       string    `json:"jti"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// load 以迭代器模式从磁盘流式读取吊销条目，丢弃已过期的记录
func (r *RevocationRepository) load() iter.Seq2[revocationEntry, error] {
	return func(yield func(revocationEntry, error) bool) {
		file, err := os.Open(r.filename())
		if err != nil {
			if os.IsNotExist(err) {
				return
			}
			yield(revocationEntry{}, err)
			return
		}
		defer file.Close()

		dec := json.NewDecoder(file)
		now := time.Now()
		for dec.More() {
			var entry revocationEntry
			if err := dec.Decode(&entry); err != nil {
				yield(revocationEntry{}, err)
				return
			}
			// 丢弃已过期的条目，不对外 yield
			if now.Before(entry.ExpiresAt) {
				if !yield(entry, nil) {
					return
				}
			}
		}
	}
}

// Add 将 JTI 加入吊销列表，通过迭代器流式写入磁盘，避免全量收集到内存
func (r *RevocationRepository) Add(ctx context.Context, jti string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return util.AtomicSave(r.filename(), func(f *os.File) error {
		enc := json.NewEncoder(f)
		// 流式写入已有条目，load 已内置过期过滤
		for entry, err := range r.load() {
			if err != nil {
				return err
			}
			if err := enc.Encode(entry); err != nil {
				return err
			}
		}
		// 追加新条目
		return enc.Encode(revocationEntry{JTI: jti, ExpiresAt: expiresAt})
	})
}

// IsRevoked 检查指定 JTI 是否存在于吊销列表中，利用迭代器提前退出
func (r *RevocationRepository) IsRevoked(ctx context.Context, jti string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for entry, err := range r.load() {
		if err != nil {
			return false, err
		}
		if entry.JTI == jti {
			return true, nil
		}
	}
	return false, nil
}

// FindAll 以迭代器返回所有有效的吊销条目
func (r *RevocationRepository) FindAll(ctx context.Context) iter.Seq2[revocationEntry, error] {
	return func(yield func(revocationEntry, error) bool) {
		var entries []revocationEntry
		var loadErr error

		func() {
			r.mu.RLock()
			defer r.mu.RUnlock()
			for entry, err := range r.load() {
				if err != nil {
					loadErr = err
					return
				}
				entries = append(entries, entry)
			}
		}()

		if loadErr != nil {
			yield(revocationEntry{}, loadErr)
			return
		}

		for _, entry := range entries {
			if !yield(entry, nil) {
				return
			}
		}
	}
}

// #endregion

// #region CachedRevocationList 带内存缓存的吊销列表装饰器

// CachedRevocationList 为 device.RevocationList 提供基于内存的缓存装饰器
type CachedRevocationList struct {
	repo    *RevocationRepository
	entries map[string]time.Time // jti -> expiresAt
	mu      sync.RWMutex
}

// NewCachedRevocationList 创建并从底层仓库初始化一个 CachedRevocationList 实例
func NewCachedRevocationList(ctx context.Context, repo *RevocationRepository) (*CachedRevocationList, error) {
	c := &CachedRevocationList{
		repo:    repo,
		entries: make(map[string]time.Time),
	}

	// 首次启动时，全量加载底层存储中的吊销列表到内存缓存，丢弃已过期条目
	for entry, err := range repo.FindAll(ctx) {
		if err != nil {
			return nil, err
		}
		c.entries[entry.JTI] = entry.ExpiresAt
	}

	return c, nil
}

// Add 将 JTI 加入吊销列表，持久化并更新内存缓存，同时丢弃已过期条目
func (c *CachedRevocationList) Add(ctx context.Context, jti string, expiresAt time.Time) error {
	// 先持久化到磁盘
	if err := c.repo.Add(ctx, jti, expiresAt); err != nil {
		return err
	}

	// 更新内存缓存，同时清理已过期条目
	c.mu.Lock()
	c.entries[jti] = expiresAt
	now := time.Now()
	for existingJTI, exp := range c.entries {
		if !now.Before(exp) {
			delete(c.entries, existingJTI)
		}
	}
	c.mu.Unlock()
	return nil
}

// IsRevoked 从内存缓存中 O(1) 检查 JTI 是否已吊销
func (c *CachedRevocationList) IsRevoked(ctx context.Context, jti string) (bool, error) {
	c.mu.RLock()
	_, ok := c.entries[jti]
	c.mu.RUnlock()
	return ok, nil
}

// 编译时接口检查，确保 CachedRevocationList 完整实现了 device.RevocationList 接口
var _ device.RevocationList = (*CachedRevocationList)(nil)

// #endregion
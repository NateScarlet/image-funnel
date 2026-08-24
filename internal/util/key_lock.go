package util

import (
	"sync"
)

type keyLockEntry struct {
	mu  sync.Mutex
	ref int // 引用计数，跟踪当前有多少个 goroutine 正在使用或等待该 key
}

var keyLockEntryPool = sync.Pool{
	New: func() any {
		return &keyLockEntry{}
	},
}

const keyLockShardSize = 256

// KeyLock 是一个基于 key 的分片互斥锁，支持细粒度的并发控制
type KeyLock struct {
	shards   [keyLockShardSize]*keyLockShard
	initOnce sync.Once
}

func (kl *KeyLock) init() {
	kl.initOnce.Do(func() {
		for index := range kl.shards {
			kl.shards[index] = &keyLockShard{
				locks: map[string]*keyLockEntry{},
			}
		}
	})
}

// FNV-1a 32位哈希常数
const (
	offset32 = 2166136261
	prime32  = 16777619
)

// 手写内联 fnv1a 算法，直接在栈上对 string 的字节进行哈希，避免哈希对象分配
func fnv1a(key string) uint32 {
	hash := uint32(offset32)
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= prime32
	}
	return hash
}

// Lock 获取某个 key 的互斥锁。如果已被其他 goroutine 持有，当前 goroutine 将阻塞直到获取成功。
func (kl *KeyLock) Lock(key string) {
	kl.init()
	idx := fnv1a(key) % keyLockShardSize
	shard := kl.shards[idx]

	shard.global.Lock()
	entry, ok := shard.locks[key]
	if !ok {
		// 从 pool 租用锁实体，避免在 map 频繁创建和销毁锁导致内存碎片与 GC 压力
		entry = keyLockEntryPool.Get().(*keyLockEntry)
		shard.locks[key] = entry
	}
	entry.ref++
	shard.global.Unlock()

	// 阻塞在此直到成功持有该 key 的互斥锁
	entry.mu.Lock()
}

// Unlock 释放某个 key 的互斥锁。
func (kl *KeyLock) Unlock(key string) {
	kl.init()
	idx := fnv1a(key) % keyLockShardSize
	shard := kl.shards[idx]

	shard.global.Lock()
	entry, ok := shard.locks[key]
	if !ok {
		shard.global.Unlock()
		return
	}
	entry.ref--
	ref := entry.ref
	if ref == 0 {
		// 当没有任何 goroutine 等待该 key 时，将其从分片 Map 中删除，腾出空间
		delete(shard.locks, key)
	}
	shard.global.Unlock()

	entry.mu.Unlock()
	if ref == 0 {
		// 仅在完全没有引用时，才将实体归还 pool 以供其他 key 复用
		keyLockEntryPool.Put(entry)
	}
}

type keyLockShard struct {
	global sync.Mutex
	locks  map[string]*keyLockEntry
}

package util

import (
	"hash/fnv"
	"sync"
)

type keyLockEntry struct {
	mu  sync.Mutex
	ref int // 引用计数
}

var keyLockEntryPool = sync.Pool{
	New: func() any {
		return &keyLockEntry{}
	},
}

const keyLockShardSize = 256

type KeyLock struct {
	shards   [keyLockShardSize]*keyLockShard
	initOnce sync.Once
}

func (kl *KeyLock) init() {
	kl.initOnce.Do(func() {
		for index := range kl.shards {
			var shard = &keyLockShard{
				locks: map[string]*keyLockEntry{},
			}
			kl.shards[index] = shard
		}
	})
}

// Lock 获取某个 key 的互斥锁，返回解锁函数，解锁非并发安全
func (kl *KeyLock) Lock(key string) func() {
	kl.init()
	var h = fnv.New32a()
	h.Write([]byte(key))
	return kl.shards[h.Sum32()%keyLockShardSize].Lock(key)
}

type keyLockShard struct {
	global sync.Mutex
	locks  map[string]*keyLockEntry
}

func (kl *keyLockShard) Lock(key string) func() {
	kl.global.Lock()
	entry, ok := kl.locks[key]
	if !ok {
		entry = keyLockEntryPool.Get().(*keyLockEntry)
		kl.locks[key] = entry
	}
	entry.ref++
	kl.global.Unlock()

	entry.mu.Lock()
	return func() {
		kl.global.Lock()
		entry.ref--
		if entry.ref == 0 {
			delete(kl.locks, key)
		}
		kl.global.Unlock()

		entry.mu.Unlock()
		if entry.ref == 0 {
			keyLockEntryPool.Put(entry)
		}
	}
}

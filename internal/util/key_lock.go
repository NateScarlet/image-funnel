package util

import "sync"

type keyLockEntry struct {
	mu  sync.Mutex
	ref int // 引用计数
}

type KeyLock struct {
	global sync.Mutex
	locks  map[string]*keyLockEntry
}

// Lock 获取某个 key 的互斥锁，返回解锁函数，重复解锁无作用
func (kl *KeyLock) Lock(key string) func() {
	kl.global.Lock()
	if kl.locks == nil {
		kl.locks = make(map[string]*keyLockEntry)
	}
	entry, ok := kl.locks[key]
	if !ok {
		entry = &keyLockEntry{}
		kl.locks[key] = entry
	}
	entry.ref++
	kl.global.Unlock()

	entry.mu.Lock()

	return sync.OnceFunc(func() {
		kl.global.Lock()
		entry.ref--
		if entry.ref == 0 {
			delete(kl.locks, key)
		}
		kl.global.Unlock()

		entry.mu.Unlock()
	})
}

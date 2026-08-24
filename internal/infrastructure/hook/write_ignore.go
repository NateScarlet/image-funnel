package hook

import (
	"os"
	"time"

	"github.com/cespare/xxhash/v2"
)

func (r *Runner) hashContent(content []byte) uint64 {
	return xxhash.Sum64(content)
}

func (r *Runner) addWriteIgnore(absPath string, contentHash uint64, duration time.Duration) {
	r.muIgnore.Lock()
	defer r.muIgnore.Unlock()

	r.writeIgnore[absPath] = writeIgnoreItem{
		contentHash: contentHash,
		expireTime:  time.Now().Add(duration),
	}

	// 轻量清理过期的忽略哈希项
	now := time.Now()
	for path, item := range r.writeIgnore {
		if now.After(item.expireTime) {
			delete(r.writeIgnore, path)
		}
	}
}

func (r *Runner) shouldIgnoreEvent(absPath string, content []byte) bool {
	r.muIgnore.Lock()
	defer r.muIgnore.Unlock()

	item, exists := r.writeIgnore[absPath]
	if !exists {
		return false
	}
	if time.Now().After(item.expireTime) {
		delete(r.writeIgnore, absPath)
		return false
	}

	return item.contentHash == r.hashContent(content)
}

// writeFileWithIgnore 写入文件前注册防重入哈希，避免自身写入触发文件变更事件
func (r *Runner) writeFileWithIgnore(absPath string, content []byte, perm os.FileMode) error {
	r.addWriteIgnore(absPath, r.hashContent(content), 10*time.Second)
	return os.WriteFile(absPath, content, perm)
}

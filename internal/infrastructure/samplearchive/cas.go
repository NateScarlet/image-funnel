package samplearchive

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// CID 返回原始字节的 CIDv1（codec raw / sha2-256 multihash）的 base32 大写形式。
// 首字符固定为多基编码前缀 B，后续字符即存储布局中使用的 h。
func CID(data []byte) string {
	digest := sha256.Sum256(data)

	buf := make([]byte, 0, 4+len(digest))
	buf = binary.AppendUvarint(buf, 1)    // CID version 1
	buf = binary.AppendUvarint(buf, 0x55) // codec: raw
	buf = binary.AppendUvarint(buf, 0x12) // multihash: sha2-256
	buf = binary.AppendUvarint(buf, 32)   // digest 长度
	buf = append(buf, digest[:]...)

	return "B" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
}

// shardFromCID 分片目录取 h 的倒数第 3、2 位字符，与用户其它 IPFS 兼容 CAS 项目一致。
func shardFromCID(cid string) string {
	h := cid[1:]
	return h[len(h)-3 : len(h)-1]
}

// CAS 内容寻址存储：文件写入 root/<分片>/<h>.data，相同字节只落盘一次。
type CAS struct {
	root string
}

func NewCAS(root string) *CAS {
	return &CAS{root: root}
}

// Put 写入缩略图字节并返回其 CID。
func (c *CAS) Put(data []byte) (string, error) {
	cid := CID(data)
	h := cid[1:]
	dir := filepath.Join(c.root, shardFromCID(cid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create CAS shard directory: %w", err)
	}

	path := filepath.Join(dir, h+".data")
	if _, err := os.Stat(path); err == nil {
		return cid, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat CAS object: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write CAS object: %w", err)
	}
	return cid, nil
}

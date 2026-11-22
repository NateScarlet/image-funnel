package imageversion

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fakeMeta 是从 stat 元数据中截取的最小视图，便于精确控制亚秒级修改时间
type fakeMeta struct {
	modTime time.Time
	size    int64
}

func (m fakeMeta) ModTime() time.Time { return m.modTime }
func (m fakeMeta) Size() int64        { return m.size }

func TestNew_Views(t *testing.T) {
	// 秒级与纳秒级分量直接写在字面量里，避免用被测逻辑自身的计算副本作为期望值
	base := time.Unix(1_764_300_000, 123_456_789)

	t.Run("相同元数据得到相同视图", func(t *testing.T) {
		first := New(fakeMeta{modTime: base, size: 1024})
		second := New(fakeMeta{modTime: base, size: 1024})
		assert.Equal(t, first.Second(), second.Second())
		assert.Equal(t, first.Nano(), second.Nano())
	})

	t.Run("视图精度由共享定义决定", func(t *testing.T) {
		v := New(fakeMeta{modTime: base, size: 1024})
		assert.Equal(t, SecondView{ModUnix: 1_764_300_000, Size: 1024}, v.Second())
		assert.Equal(t, NanoView{ModNano: 1_764_300_000_123_456_789, Size: 1024}, v.Nano())
	})

	t.Run("仅亚秒级修改时间变化", func(t *testing.T) {
		// 同一秒内仅纳秒不同：签名用的秒级视图必须相同，变体用的纳秒视图必须不同
		first := New(fakeMeta{modTime: base, size: 1024})
		second := New(fakeMeta{modTime: base.Add(time.Millisecond), size: 1024})
		assert.Equal(t, first.Second(), second.Second())
		assert.NotEqual(t, first.Nano(), second.Nano())
	})

	t.Run("大小变化", func(t *testing.T) {
		first := New(fakeMeta{modTime: base, size: 1024})
		second := New(fakeMeta{modTime: base, size: 2048})
		assert.NotEqual(t, first.Second(), second.Second())
		assert.NotEqual(t, first.Nano(), second.Nano())
	})
}

package transcode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEffectiveWidth(t *testing.T) {
	// 不放大规则的唯一实现：0 表示无宽度诉求 / 源宽不可知，两者均 >0 时取较小值
	tests := []struct {
		name        string
		width       int
		sourceWidth int
		want        int
	}{
		{"无宽度诉求", 0, 0, 0},
		{"无宽度诉求且源宽已知", 0, 2048, 0},
		{"源宽不可知时保持请求宽度", 4096, 0, 4096},
		{"源宽不可知时保持小宽度", 1024, 0, 1024},
		{"请求宽于源图收缩到源宽", 4096, 2048, 2048},
		{"请求窄于源图保持请求宽度", 1024, 2048, 1024},
		{"等宽保持不变", 2048, 2048, 2048},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, EffectiveWidth(tt.width, tt.sourceWidth))
		})
	}
}

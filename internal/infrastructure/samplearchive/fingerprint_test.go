package samplearchive_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"main/internal/infrastructure/samplearchive"
)

func TestFingerprint_CollectsAllStringValuesSortedDeduplicated(t *testing.T) {
	// 字符串只来自值，不来自键；节点/字段名是任意的，不能硬编码
	prompt := []byte(`{"node_a":{"class_type":123,"inputs":{"text":"world","seed":42}},"node_b":{"inputs":["hello","world"]}}`)

	// 独立期望值：SHA256("hello\nworld")，由 pwsh 预先算出
	assert.Equal(t, "26c60a61d01db5836ca70fefd44a6a016620413c8ef5f259a6c5612d4f79d3b8",
		samplearchive.Fingerprint(prompt))
}

func TestFingerprint_IndependentOfKeyAndValueOrder(t *testing.T) {
	left := []byte(`{"a":"world","b":"hello"}`)
	right := []byte(`{"z":"hello","y":"world"}`)

	assert.Equal(t, samplearchive.Fingerprint(left), samplearchive.Fingerprint(right))
}

func TestFingerprint_IgnoresNumbersAndBooleans(t *testing.T) {
	// seed / steps / cfg 是数字，被自然忽略；同配置不同种子得到相同指纹
	withSeed := []byte(`{"text":"x","seed":12345,"steps":20,"cfg":7.5,"flag":true}`)

	// 独立期望值：SHA256("x")
	assert.Equal(t, "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881",
		samplearchive.Fingerprint(withSeed))
}

func TestFingerprint_WorksWithArbitraryNestedKeys(t *testing.T) {
	// 自定义节点的任意字段/层级中的字符串都应被收集
	prompt := []byte(`{"custom_xyz":{"free_form_field":"hello"},"another":{"deep":{"nested":"world"}}}`)

	assert.Equal(t, "26c60a61d01db5836ca70fefd44a6a016620413c8ef5f259a6c5612d4f79d3b8",
		samplearchive.Fingerprint(prompt))
}

func TestFingerprint_EmptyWhenNoStringsOrInvalidJSON(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prompt []byte
	}{
		{name: "nil", prompt: nil},
		{name: "empty", prompt: []byte("")},
		{name: "whitespace", prompt: []byte("   \n")},
		{name: "invalid json", prompt: []byte(`{"a":`)},
		{name: "numbers only", prompt: []byte(`{"seed":1,"steps":2}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, samplearchive.Fingerprint(tc.prompt))
		})
	}
}

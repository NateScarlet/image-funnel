package samplearchive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
)

// Fingerprint 从完整 prompt JSON 提取启发式指纹：
// 递归收集所有字符串类型的值，排序去重后以换行连接并计算 SHA-256。
//
// 局限（必须在数据规范中显式标注）：
//   - 字符串里可能混入节点类型名、模型名、节点 ID、自定义节点写入的随机串或临时路径，
//     因此同一配置可能得到不同指纹，不同配置也可能被归为一组；
//   - 数字（种子/steps/cfg/尺寸）被自然忽略，这是与 CID「字节完全相同」互补的分组维度；
//   - 无元数据、解析失败或没有任何字符串时返回空字符串，不参与指纹分组。
//
// 不假设任何节点类型或字段名，仅依据 JSON 值的类型工作。
func Fingerprint(promptJSON []byte) string {
	if len(bytes.TrimSpace(promptJSON)) == 0 {
		return ""
	}

	var value any
	if err := json.Unmarshal(promptJSON, &value); err != nil {
		return ""
	}

	var values []string
	collectStrings(value, &values)
	if len(values) == 0 {
		return ""
	}

	slices.Sort(values)
	values = slices.Compact(values)
	sum := sha256.Sum256([]byte(strings.Join(values, "\n")))
	return hex.EncodeToString(sum[:])
}

// collectStrings 递归遍历 JSON 解码后的值，只收集字符串值；数字、布尔、null 被忽略。
func collectStrings(value any, out *[]string) {
	switch v := value.(type) {
	case string:
		*out = append(*out, v)
	case map[string]any:
		for _, item := range v {
			collectStrings(item, out)
		}
	case []any:
		for _, item := range v {
			collectStrings(item, out)
		}
	}
}

package samplearchive

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ReadRecords 流式读取 append-only JSONL；归档不存在时返回空集合。
func ReadRecords(dataRoot string) ([]Record, error) {
	file, err := os.Open(filepath.Join(dataRoot, "samples.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open sample archive: %w", err)
	}
	defer file.Close()

	var records []Record
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("decode sample archive line %d: %w", len(records)+1, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan sample archive: %w", err)
	}
	return records, nil
}

// LatestPerImage 按图片标识只保留 timestamp 最大的一条（「留到最后」的决策）；
// timestamp 相同时以轮次较大者为准。结果按图片标识排序，保证确定性。
func LatestPerImage(records []Record) []Record {
	latest := make(map[string]Record, len(records))
	for _, record := range records {
		existing, ok := latest[record.ImageID]
		if !ok || record.Timestamp.After(existing.Timestamp) ||
			(record.Timestamp.Equal(existing.Timestamp) && record.Round >= existing.Round) {
			latest[record.ImageID] = record
		}
	}

	result := make([]Record, 0, len(latest))
	for _, record := range latest {
		result = append(result, record)
	}
	slices.SortFunc(result, func(a, b Record) int { return strings.Compare(a.ImageID, b.ImageID) })
	return result
}

// GroupKey 返回分组键：字符串指纹可用时优先用它，否则回退 session id。
//
// 局限：指纹是启发式（可能被节点 ID / 随机串 / 临时路径污染），session id 只表示
// 同批筛选而非同配置（remix 会在提交后按评分编辑生成参数），二者都不是精确的
// 「同提示词」语义。报告中必须标注这一点。
func GroupKey(record Record) string {
	if record.Fingerprint != "" {
		return "fp:" + record.Fingerprint
	}
	return "session:" + record.SessionID
}

// Split 按分组键把记录切分为 train / test，同一分组一定落在同一侧，禁止随机切分。
// testRatio 为测试集目标比例；按分组键排序后整组贪心分配，因分组粒度无法精确命中比例。
func Split(records []Record, testRatio float64) (train, test []Record) {
	groups := make(map[string][]Record)
	for _, record := range records {
		key := GroupKey(record)
		groups[key] = append(groups[key], record)
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	target := int(math.Round(float64(len(records)) * testRatio))
	for _, key := range keys {
		group := groups[key]
		if len(test)+len(group) <= target || (len(train) == 0 && len(test) == 0) {
			test = append(test, group...)
			continue
		}
		train = append(train, group...)
	}
	return train, test
}

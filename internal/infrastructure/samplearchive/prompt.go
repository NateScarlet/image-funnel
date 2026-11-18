package samplearchive

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"os"
)

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// PNGPromptReader 从 PNG 文本块中尽力读取 ComfyUI prompt JSON；读不到返回 nil。
// 这是有意的尽力而为：JPEG 等非 PNG 图片没有该元数据，不应因此阻断提交。
type PNGPromptReader struct{}

func (PNGPromptReader) Prompt(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return PromptFromPNG(data)
}

// PromptFromPNG 读取名为 prompt 的 tEXt / zTXt / iTXt 块值。
func PromptFromPNG(data []byte) []byte {
	if len(data) < len(pngSignature) || !bytes.Equal(data[:len(pngSignature)], pngSignature) {
		return nil
	}

	pos := len(pngSignature)
	for pos+12 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		if length < 0 || pos+12+length > len(data) {
			return nil
		}
		chunkType := string(data[pos+4 : pos+8])
		payload := data[pos+8 : pos+8+length]

		if text, ok := promptTextChunk(chunkType, payload); ok {
			return text
		}

		pos += 12 + length
		if chunkType == "IEND" {
			break
		}
	}
	return nil
}

// promptTextChunk 解析单个文本块；只认 prompt 关键字，不假设任何节点类型。
func promptTextChunk(chunkType string, payload []byte) ([]byte, bool) {
	switch chunkType {
	case "tEXt":
		keyword, value, ok := bytes.Cut(payload, []byte{0})
		if ok && string(keyword) == "prompt" {
			return value, true
		}
	case "zTXt":
		keyword, rest, ok := bytes.Cut(payload, []byte{0})
		if !ok || string(keyword) != "prompt" || len(rest) == 0 || rest[0] != 0 {
			return nil, false
		}
		return inflate(rest[1:]), true
	case "iTXt":
		keyword, rest, ok := bytes.Cut(payload, []byte{0})
		if !ok || string(keyword) != "prompt" || len(rest) < 2 || rest[1] != 0 {
			return nil, false
		}
		// 语言标签与翻译后的关键字，均为 null 结尾
		_, rest, ok = bytes.Cut(rest[2:], []byte{0})
		if !ok {
			return nil, false
		}
		_, rest, ok = bytes.Cut(rest, []byte{0})
		if !ok || len(rest) == 0 {
			return nil, false
		}
		if rest[0] == 1 {
			return inflate(rest[1:]), true
		}
		return rest, true
	}
	return nil, false
}

// inflate 解压失败等同于元数据不可解析：PromptReader 是尽力而为端口，
// 返回 nil 让指纹留空，不向 Commit 传播错误，避免损坏元数据阻断标注写入。
func inflate(data []byte) []byte {
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	defer reader.Close()
	out, err := io.ReadAll(reader)
	if err != nil {
		return nil
	}
	return out
}

var _ PromptReader = PNGPromptReader{}

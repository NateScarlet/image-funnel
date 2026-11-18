package samplearchive_test

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"main/internal/infrastructure/samplearchive"
)

// pngWithTextChunk 在最小 PNG 的 IHDR 之后插入一个文本块，模拟 ComfyUI 写入的元数据。
func pngWithTextChunk(t *testing.T, chunkType string, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	raw := buf.Bytes()

	// PNG 签名 8 字节 + IHDR（长度 4 + 类型 4 + 数据 13 + CRC 4）
	const afterIHDR = 8 + 25
	out := append([]byte{}, raw[:afterIHDR]...)

	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(payload)))
	out = append(out, length[:]...)
	out = append(out, []byte(chunkType)...)
	out = append(out, payload...)

	checksum := crc32.NewIEEE()
	_, _ = checksum.Write([]byte(chunkType))
	_, _ = checksum.Write(payload)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], checksum.Sum32())
	out = append(out, crc[:]...)

	return append(out, raw[afterIHDR:]...)
}

func TestPromptFromPNG_ReadsTextPrompt(t *testing.T) {
	data := pngWithTextChunk(t, "tEXt", []byte("prompt\x00"+`{"text":"hello"}`))

	assert.Equal(t, []byte(`{"text":"hello"}`), samplearchive.PromptFromPNG(data))
}

func TestPromptFromPNG_ReadsCompressedTextPrompt(t *testing.T) {
	compressed := compressZlib(t, []byte(`{"text":"hello"}`))
	payload := append([]byte("prompt\x00\x00"), compressed...)
	data := pngWithTextChunk(t, "zTXt", payload)

	assert.Equal(t, []byte(`{"text":"hello"}`), samplearchive.PromptFromPNG(data))
}

func TestPromptFromPNG_MissingMetadataReturnsNil(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))))

	assert.Nil(t, samplearchive.PromptFromPNG(buf.Bytes()))
	assert.Nil(t, samplearchive.PromptFromPNG([]byte("not a png")))
}

func compressZlib(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zlib.NewWriter(&buf)
	_, err := writer.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

func TestPNGPromptReader_ReadsFromDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	data := pngWithTextChunk(t, "tEXt", []byte("prompt\x00"+`{"text":"hello"}`))
	require.NoError(t, os.WriteFile(path, data, 0644))

	assert.Equal(t, []byte(`{"text":"hello"}`), samplearchive.PNGPromptReader{}.Prompt(path))
}

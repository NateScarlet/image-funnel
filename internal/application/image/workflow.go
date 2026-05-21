package image

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExtractComfyUIWorkflow 从 PNG 文件中提取 ComfyUI 工作流
func ExtractComfyUIWorkflow(absPath string) (*string, error) {
	// 只处理 PNG 文件
	if !strings.EqualFold(filepath.Ext(absPath), ".png") {
		return nil, nil
	}

	file, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// 读取 PNG 文件头
	header := make([]byte, 8)
	if _, err := file.Read(header); err != nil {
		return nil, err
	}

	// 检查 PNG 签名
	if !bytes.Equal(header, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return nil, fmt.Errorf("not a valid PNG file")
	}

	// 遍历 PNG 块
	for {
		// 读取块长度（4 字节，大端序）
		var length uint32
		if err := binary.Read(file, binary.BigEndian, &length); err != nil {
			return nil, err
		}

		// 读取块类型（4 字节）
		chunkType := make([]byte, 4)
		if _, err := file.Read(chunkType); err != nil {
			return nil, err
		}

		// 读取块数据
		data := make([]byte, length)
		if _, err := file.Read(data); err != nil {
			return nil, err
		}

		// 跳过 CRC（4 字节）
		crc := make([]byte, 4)
		if _, err := file.Read(crc); err != nil {
			return nil, err
		}

		// 检查是否为 tEXt 块
		if string(chunkType) == "tEXt" {
			// tEXt 块格式：关键字 + 0x00 + 数据
			split := bytes.SplitN(data, []byte{0x00}, 2)
			if len(split) == 2 {
				keyword := string(split[0])
				if keyword == "workflow" {
					workflow := string(split[1])
					return &workflow, nil
				}
			}
		}

		// 检查是否为 IEND 块（文件结束）
		if string(chunkType) == "IEND" {
			break
		}
	}

	return nil, nil
}

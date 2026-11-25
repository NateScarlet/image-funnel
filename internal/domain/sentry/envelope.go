package sentry

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrNoEventItem 表示 envelope 中没有事件项（如 SDK 的 client_report）。
var ErrNoEventItem = errors.New("envelope contains no event item")

// Envelope 是从 Sentry envelope 中提取出的事件信息，仅用于指纹计算与文件命名。
// 原始 envelope 字节由落盘组件原样保存。
type Envelope struct {
	EventID   string
	EventJSON []byte
}

// ParseEnvelope 按 envelope 的逐行格式提取 envelope 头与第一个事件项。
// 事件项声明 length 时按字节读取，未声明时读取整行。
func ParseEnvelope(data []byte) (*Envelope, error) {
	reader := bufio.NewReader(bytes.NewReader(data))
	headerLine, err := readEnvelopeLine(reader)
	if err != nil {
		return nil, fmt.Errorf("read envelope header: %w", err)
	}
	var header struct {
		EventID string `json:"event_id"`
	}
	if err := json.Unmarshal(headerLine, &header); err != nil {
		return nil, fmt.Errorf("invalid envelope header: %w", err)
	}

	parsed := &Envelope{EventID: sanitize(header.EventID)}
	for {
		itemHeader, err := readEnvelopeLine(reader)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read envelope item header: %w", err)
		}
		var item struct {
			Type   string `json:"type"`
			Length int    `json:"length"`
		}
		if err := json.Unmarshal(itemHeader, &item); err != nil {
			return nil, fmt.Errorf("invalid envelope item header: %w", err)
		}

		payload, err := readEnvelopePayload(reader, item.Length)
		if err != nil {
			return nil, err
		}
		if item.Type == "event" && parsed.EventJSON == nil {
			if !json.Valid(payload) {
				return nil, errors.New("envelope event item is not valid JSON")
			}
			parsed.EventJSON = payload
		}
	}

	if parsed.EventJSON == nil {
		return nil, ErrNoEventItem
	}
	return parsed, nil
}

func readEnvelopeLine(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(line) == 0 {
		return nil, io.EOF
	}
	return bytes.TrimRight(line, "\r\n"), nil
}

func readEnvelopePayload(reader *bufio.Reader, length int) ([]byte, error) {
	if length > 0 {
		payload := make([]byte, length)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return nil, fmt.Errorf("read envelope item payload: %w", err)
		}
		// length 之后紧跟换行，消费掉以免被当作下一项头； payload 位于末尾时 EOF 是正常结束
		if _, err := reader.ReadBytes('\n'); err != nil && err != io.EOF {
			return nil, fmt.Errorf("read envelope item delimiter: %w", err)
		}
		return payload, nil
	}
	payload, err := readEnvelopeLine(reader)
	if err != nil {
		return nil, fmt.Errorf("read envelope item payload: %w", err)
	}
	return payload, nil
}

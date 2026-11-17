package image

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

// clipboardStub 是剪贴板端口的空实现，仅实现真正跨 seam 使用的两个方法，
// 用于验证应用层在剪贴板失败时降级为 supported=false 的行为。
type clipboardStub struct {
	html     string
	readErr  error
	addCalls int
}

func (s *clipboardStub) ReadHTMLFormat() (string, error) {
	return s.html, s.readErr
}

func (s *clipboardStub) AddFiles([]string, string) error {
	s.addCalls++
	return nil
}

func TestAttachFileToClipboard_NonceMismatch(t *testing.T) {
	// 注入返回错误 nonce 的空实现，覆盖 nonce 不匹配返回 supported=false 的路径
	stub := &clipboardStub{
		html: `<html><head><meta name="` + clipboardTokenMetaName + `" content="from-clipboard"/></head><body></body></html>`,
	}
	h := &Handler{clipboard: stub, logger: zap.NewNop()}

	result, err := h.AttachFileToClipboard(context.Background(), []string{"test.png"}, "expected-nonce")
	if err != nil {
		t.Fatalf("AttachFileToClipboard returned error: %v", err)
	}
	if result == nil {
		t.Fatal("AttachFileToClipboard returned nil result")
	}
	if result.Supported {
		t.Error("nonce mismatch should map to supported=false")
	}
	if stub.addCalls != 0 {
		t.Errorf("AddFiles should not be called on nonce mismatch, got %d calls", stub.addCalls)
	}
}

func TestAttachFileToClipboard_ReadHTMLFailed(t *testing.T) {
	// 非 Windows 适配器直接返回「不支持」错误，应经既有错误路径映射为 supported=false
	stub := &clipboardStub{readErr: errors.New("not supported on this platform")}
	h := &Handler{clipboard: stub, logger: zap.NewNop()}

	result, err := h.AttachFileToClipboard(context.Background(), []string{"test.png"}, "nonce")
	if err != nil {
		t.Fatalf("AttachFileToClipboard returned error: %v", err)
	}
	if result == nil {
		t.Fatal("AttachFileToClipboard returned nil result")
	}
	if result.Supported {
		t.Error("clipboard read failure should map to supported=false")
	}
	if stub.addCalls != 0 {
		t.Errorf("AddFiles should not be called when clipboard read fails, got %d calls", stub.addCalls)
	}
}

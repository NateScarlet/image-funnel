package image

import "testing"

func TestExtractClipboardNonce_Valid(t *testing.T) {
	html := `<html><head><meta name="io.github.natescarlet.image-funnel.nonce" content="abc-123"/></head><body><pre>C:\test.png</pre></body></html>`
	nonce := extractClipboardNonce(html)
	if nonce != "abc-123" {
		t.Errorf("expected %q, got %q", "abc-123", nonce)
	}
}

func TestExtractClipboardNonce_WithAttributesBefore(t *testing.T) {
	html := `<html><head><meta charset="utf-8"><meta name="io.github.natescarlet.image-funnel.nonce" content="uuid-456"/></head><body></body></html>`
	nonce := extractClipboardNonce(html)
	if nonce != "uuid-456" {
		t.Errorf("expected %q, got %q", "uuid-456", nonce)
	}
}

func TestExtractClipboardNonce_NoMetaTag(t *testing.T) {
	html := `<html><head></head><body><pre>hello</pre></body></html>`
	nonce := extractClipboardNonce(html)
	if nonce != "" {
		t.Errorf("expected empty string, got %q", nonce)
	}
}

func TestExtractClipboardNonce_EmptyHTML(t *testing.T) {
	nonce := extractClipboardNonce("")
	if nonce != "" {
		t.Errorf("expected empty string, got %q", nonce)
	}
}

func TestExtractClipboardNonce_RealWorldFormat(t *testing.T) {
	// 模拟 Chromium 生成的 CF_HTML 格式（去除头部后的纯 HTML）
	html := `<html><head><meta name="io.github.natescarlet.image-funnel.nonce" content="e0d0db63-7c59-4b51-8574-ade30e5f127c"/></head><body><pre>C:\Workspaces\image-funnel\data.local\test.png</pre></body></html>`
	nonce := extractClipboardNonce(html)
	if nonce != "e0d0db63-7c59-4b51-8574-ade30e5f127c" {
		t.Errorf("expected UUID, got %q", nonce)
	}
}

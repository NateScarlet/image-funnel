package memo

import (
	"main/internal/scalar"
	"testing"
)

func TestParseMemoContent(t *testing.T) {
	// 定义测试用例结构体
	tests := []struct {
		name           string
		raw            string
		expectedHidden bool
		expectedBody   string
	}{
		{
			name:           "纯文本且无 frontmatter",
			raw:            "这是一个普通的备忘录内容。\n第二行。",
			expectedHidden: false,
			expectedBody:   "这是一个普通的备忘录内容。\n第二行。",
		},
		{
			name: "包含 frontmatter 但不含有隐藏字段",
			raw: `---
title: 测试备忘录
tags: [test, info]
---
正文内容在此。`,
			expectedHidden: false,
			expectedBody:   "正文内容在此。",
		},
		{
			name: "包含 frontmatter 且设置 hidden 为 true",
			raw: `---
hidden: true
---
正文内容在此。`,
			expectedHidden: true,
			expectedBody:   "正文内容在此。",
		},
		{
			name: "包含 frontmatter 且设置 hide 为 true",
			raw: `---
hide: true
---
正文内容在此。`,
			expectedHidden: true,
			expectedBody:   "正文内容在此。",
		},
		{
			name: "包含 frontmatter 且设置 hidden 为 false",
			raw: `---
hidden: false
---
正文内容在此。`,
			expectedHidden: false,
			expectedBody:   "正文内容在此。",
		},
		{
			name: "Frontmatter 属性名与属性值大小写混合且包含注释与空行",
			raw: `---
# 这是一个注释
HiDe:   tRuE
  
---
正文内容在此。`,
			expectedHidden: true,
			expectedBody:   "正文内容在此。",
		},
		{
			name: "未闭合的 frontmatter 应该作为普通文本",
			raw: `---
hidden: true
正文内容在此。`,
			expectedHidden: false,
			expectedBody:   "---\nhidden: true\n正文内容在此。",
		},
		{
			name:           "Windows 换行符（CRLF）的正确解析处理",
			raw:            "---\r\nhidden: true\r\n---\r\nWindows 风格换行正文。",
			expectedHidden: true,
			expectedBody:   "Windows 风格换行正文。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hidden, body := ParseContent(tt.raw)
			if hidden != tt.expectedHidden {
				t.Errorf("ParseMemoContent() hidden = %v, 想要 %v", hidden, tt.expectedHidden)
			}
			if body != tt.expectedBody {
				t.Errorf("ParseMemoContent() body = %q, 想要 %q", body, tt.expectedBody)
			}
		})
	}
}

func TestMemoCreation(t *testing.T) {
	// 测试 NewMemo 是否正确封装了 ParseMemoContent 的结果
	id := EncodeID("test.jpg.md")
	path := "/absolute/path/test.jpg.md"
	raw := "---\nhidden: true\n---\nHello World"

	m := New(id, path, raw)

	if m.ID() != id {
		t.Errorf("NewMemo().ID() = %v, 想要 %v", m.ID(), id)
	}
	if m.AbsPath() != path {
		t.Errorf("NewMemo().AbsPath() = %q, 想要 %q", m.AbsPath(), path)
	}
	if m.RawContent() != raw {
		t.Errorf("NewMemo().RawContent() = %q, 想要 %q", m.RawContent(), raw)
	}
	if m.Content() != "Hello World" {
		t.Errorf("NewMemo().Content() = %q, 想要 %q", m.Content(), "Hello World")
	}
	if !m.Hidden() {
		t.Errorf("NewMemo().Hidden() = %v, 想要 true", m.Hidden())
	}
}

func TestIDEncodingDecoding(t *testing.T) {
	relPath := "subdir/image.png.md"
	id := EncodeID(relPath)

	decoded, err := DecodeID(id)
	if err != nil {
		t.Fatalf("DecodeID 失败: %v", err)
	}
	if decoded != relPath {
		t.Errorf("DecodeID() = %q, 想要 %q", decoded, relPath)
	}

	// 测试无效 ID 解码
	_, err = DecodeID(scalar.ToID(""))
	if err == nil {
		t.Error("DecodeID 空ID 应该报错但没有")
	}

	_, err = DecodeID(scalar.ToID("invalid-prefix:abc"))
	if err == nil {
		t.Error("DecodeID 错误前缀 应该报错但没有")
	}
}

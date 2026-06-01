package memo

import (
	"context"
	"iter"
	"main/internal/apperror"
	"main/internal/scalar"
	"path/filepath"
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
	// 测试 newMemo 是否正确封装了 ParseMemoContent 的结果
	id := encodeID("test.jpg.md")
	path := "/absolute/path/test.jpg.md"
	raw := "---\nhidden: true\n---\nHello World"

	m := newMemo(id, "test.jpg.md", path, raw)

	if m.ID() != id {
		t.Errorf("newMemo().ID() = %v, 想要 %v", m.ID(), id)
	}
	if m.AbsPath() != path {
		t.Errorf("newMemo().AbsPath() = %q, 想要 %q", m.AbsPath(), path)
	}
	if m.RawContent() != raw {
		t.Errorf("newMemo().RawContent() = %q, 想要 %q", m.RawContent(), raw)
	}
	if m.Content() != "Hello World" {
		t.Errorf("newMemo().Content() = %q, 想要 %q", m.Content(), "Hello World")
	}
	if !m.Hidden() {
		t.Errorf("newMemo().Hidden() = %v, 想要 true", m.Hidden())
	}
}

func TestIDEncodingDecoding(t *testing.T) {
	relPath := "subdir/image.png.md"
	id := encodeID(relPath)

	decoded, err := decodeID(id)
	if err != nil {
		t.Fatalf("decodeID 失败: %v", err)
	}
	if decoded != relPath {
		t.Errorf("decodeID() = %q, 想要 %q", decoded, relPath)
	}

	_, err = decodeID(scalar.ToID(""))
	if err == nil {
		t.Error("decodeID 空ID 应该报错但没有")
	}
	_, err = decodeID(scalar.ToID("invalid-prefix:abc"))
	if err == nil {
		t.Error("decodeID 错误前缀 应该报错但没有")
	}
}

type mockRepo struct {
	memos map[string]*Memo // key is relPath
}

func (r *mockRepo) Read(ctx context.Context, relPath string) (*Memo, error) {
	return r.memos[relPath], nil
}

func (r *mockRepo) Write(ctx context.Context, relPath string, content string) error {
	if content == "" {
		delete(r.memos, relPath)
		return nil
	}
	r.memos[relPath] = FromRepository(relPath, relPath, content)
	return nil
}

func (r *mockRepo) Find(ctx context.Context, relPath string) iter.Seq2[*Memo, error] {
	return func(yield func(*Memo, error) bool) {}
}

func TestServiceCreate(t *testing.T) {
	repo := &mockRepo{memos: make(map[string]*Memo)}
	service := NewService(repo, "")
	ctx := context.Background()

	// 1. 测试成功创建新笔记
	m, err := service.Create(ctx, "subdir", "README.md", "测试内容")
	if err != nil {
		t.Fatalf("创建笔记失败: %v", err)
	}

	expectedID := encodeID("subdir/README.md")
	if m.ID() != expectedID {
		t.Errorf("创建出的 Memo ID = %v, 想要 %v", m.ID(), expectedID)
	}
	if m.Content() != "测试内容" {
		t.Errorf("创建出的 Memo Content = %q, 想要 %q", m.Content(), "测试内容")
	}

	// 2. 测试试图覆盖已存在的同名笔记，应返回 ALREADY_EXISTS 错误
	_, err = service.Create(ctx, "subdir", "README", "新内容")
	if err == nil {
		t.Fatal("尝试创建已存在的同名笔记，期望报错但返回了成功")
	}

	if apperror.ErrCode(err) != "ALREADY_EXISTS" {
		t.Errorf("期望的错误码是 ALREADY_EXISTS, 实际得到: %v", err)
	}
}

func TestNewEmpty(t *testing.T) {
	service := NewService(nil, "/absolute/path")
	relPath := "subdir/image.png.md"
	m := service.newEmpty(relPath)
	if m.ID() != encodeID(relPath) {
		t.Errorf("newEmpty().ID() = %v, 想要 %v", m.ID(), encodeID(relPath))
	}
	if m.RelPath() != relPath {
		t.Errorf("newEmpty().RelPath() = %q, 想要 %q", m.RelPath(), relPath)
	}
	expectedAbsPath := filepath.FromSlash("/absolute/path/subdir/image.png.md")
	if m.AbsPath() != expectedAbsPath {
		t.Errorf("newEmpty().AbsPath() = %q, 想要 %q", m.AbsPath(), expectedAbsPath)
	}
	if m.Content() != "" {
		t.Errorf("newEmpty().Content() = %q, 想要 %q", m.Content(), "")
	}
}

func TestServiceReadNonExistent(t *testing.T) {
	repo := &mockRepo{memos: make(map[string]*Memo)}
	service := NewService(repo, "/root")
	ctx := context.Background()

	relPath := "subdir/non_existent.md"
	id := encodeID(relPath)

	m, err := service.Read(ctx, id)
	if err != nil {
		t.Fatalf("Read 失败: %v", err)
	}
	if m == nil {
		t.Fatal("期望返回空 Memo 实体，但返回了 nil")
	}
	if m.ID() != id {
		t.Errorf("期望 ID 为 %v, 实际为 %v", id, m.ID())
	}
	if m.Content() != "" {
		t.Errorf("期望内容为空，实际为 %q", m.Content())
	}
	if m.AbsPath() != filepath.FromSlash("/root/subdir/non_existent.md") {
		t.Errorf("期望绝对路径为 /root/subdir/non_existent.md, 实际为 %q", m.AbsPath())
	}
}

func TestServiceReadByRelPathNonExistent(t *testing.T) {
	repo := &mockRepo{memos: make(map[string]*Memo)}
	service := NewService(repo, "/root")
	ctx := context.Background()

	relPath := "subdir/non_existent.md"
	m, err := service.ReadByRelPath(ctx, relPath)
	if err != nil {
		t.Fatalf("ReadByRelPath 失败: %v", err)
	}
	if m == nil {
		t.Fatal("期望返回空 Memo 实体，但返回了 nil")
	}
	if m.RelPath() != relPath {
		t.Errorf("期望相对路径为 %q, 实际为 %q", relPath, m.RelPath())
	}
	if m.AbsPath() != filepath.FromSlash("/root/subdir/non_existent.md") {
		t.Errorf("期望绝对路径为 /root/subdir/non_existent.md, 实际为 %q", m.AbsPath())
	}
	if m.Content() != "" {
		t.Errorf("期望内容为空，实际为 %q", m.Content())
	}
}




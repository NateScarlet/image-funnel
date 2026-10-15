package note

import (
	"context"
	"iter"
	"main/internal/apperror"
	"main/internal/domain/relpath"
	"main/internal/scalar"
	"main/internal/testutil"
	"path/filepath"
	"testing"
	"time"
)

// testRootDir 返回当前平台形态的虚拟根目录，仅用于根相对路径的围栏，不实际访问磁盘
func testRootDir() string {
	if filepath.Separator == '\\' {
		return `C:\absolute\path`
	}
	return "/absolute/path"
}

func TestParseNoteContent(t *testing.T) { // 定义测试用例结构体
	tests := []struct {
		name           string
		raw            string
		expectedHidden bool
		expectedBody   string
	}{
		{
			name:           "纯文本且无 frontmatter",
			raw:            "这是一个普通的笔记内容。\n第二行。",
			expectedHidden: false,
			expectedBody:   "这是一个普通的笔记内容。\n第二行。",
		},
		{
			name: "包含 frontmatter 但不含有隐藏字段",
			raw: `---
title: 测试笔记
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
				t.Errorf("ParseContent() hidden = %v, 想要 %v", hidden, tt.expectedHidden)
			}
			if body != tt.expectedBody {
				t.Errorf("ParseContent() body = %q, 想要 %q", body, tt.expectedBody)
			}
		})
	}
}

func TestNoteCreation(t *testing.T) {
	// 测试 Factory.New 是否正确封装了 ParseNoteContent 的结果，并进行了校验
	rootDir := testRootDir()
	raw := "---\nhidden: true\n---\nHello World"
	relPath := testutil.RelIn(t, rootDir, "test.jpg.md")

	f := NewFactory(rootDir)
	m, err := f.New(relPath, raw)
	if err != nil {
		t.Fatalf("Factory.New 失败: %v", err)
	}

	expectedID := encodeID(relPath)
	if m.ID() != expectedID {
		t.Errorf("Factory.New().ID() = %v, 想要 %v", m.ID(), expectedID)
	}
	expectedAbsPath := filepath.Join(rootDir, "test.jpg.md")
	if m.AbsPath() != expectedAbsPath {
		t.Errorf("Factory.New().AbsPath() = %q, 想要 %q", m.AbsPath(), expectedAbsPath)
	}
	if m.RawContent() != raw {
		t.Errorf("Factory.New().RawContent() = %q, 想要 %q", m.RawContent(), raw)
	}
	if m.Content() != "Hello World" {
		t.Errorf("Factory.New().Content() = %q, 想要 %q", m.Content(), "Hello World")
	}
	if !m.Hidden() {
		t.Errorf("Factory.New().Hidden() = %v, 想要 true", m.Hidden())
	}
}

func TestNoteFactoryValidation(t *testing.T) {
	rootDir := testRootDir()
	f := NewFactory(rootDir)

	// 1. relPath 不能为空：根目录本身是合法的根相对路径，但不能作为笔记文件的位置，
	// 因此空路径的拒绝是笔记的约束而非路径的约束
	_, err := f.New(relpath.Path{}, "")
	if err == nil {
		t.Error("期望 relPath 校验报错，但成功了")
	}

	// 2. relPath 必须以 .md 结尾
	_, err = f.New(testutil.RelIn(t, rootDir, "test.jpg"), "")
	if err == nil {
		t.Error("期望 relPath 缺少 .md 校验报错，但成功了")
	}
}

func TestIDEncodingDecoding(t *testing.T) {
	rootDir := testRootDir()
	relPath := testutil.RelIn(t, rootDir, "subdir/image.png.md")
	id := encodeID(relPath)

	decoded, err := decodeID(rootDir, id)
	if err != nil {
		t.Fatalf("decodeID 失败: %v", err)
	}
	if decoded != relPath {
		t.Errorf("decodeID() = %q, 想要 %q", decoded, relPath)
	}

	_, err = decodeID(rootDir, scalar.ToID(""))
	if err == nil {
		t.Error("decodeID 空ID 应该报错但没有")
	}
	_, err = decodeID(rootDir, scalar.ToID("invalid-prefix:abc"))
	if err == nil {
		t.Error("decodeID 错误前缀 应该报错但没有")
	}
}

type mockRepo struct {
	notes map[relpath.Path]*Note
}

func (r *mockRepo) Read(ctx context.Context, relPath relpath.Path) (*Note, error) {
	return r.notes[relPath], nil
}

func (r *mockRepo) Write(ctx context.Context, relPath relpath.Path, content string) error {
	if content == "" {
		delete(r.notes, relPath)
		return nil
	}
	r.notes[relPath] = FromRepository(relPath, relPath.String(), content, time.Now())
	return nil
}

func (r *mockRepo) Find(ctx context.Context, relPath relpath.Path) iter.Seq2[*Note, error] {
	return func(yield func(*Note, error) bool) {}
}

func TestServiceCreate(t *testing.T) {
	rootDir := testRootDir()
	repo := &mockRepo{notes: make(map[relpath.Path]*Note)}
	service := NewService(repo, NewFactory(rootDir), rootDir)
	ctx := context.Background()
	dirRelPath := testutil.RelIn(t, rootDir, "subdir")

	// 1. 测试成功创建新笔记
	m, err := service.Create(ctx, dirRelPath, "README.md", "测试内容")
	if err != nil {
		t.Fatalf("创建笔记失败: %v", err)
	}

	expectedID := encodeID(testutil.RelIn(t, rootDir, "subdir/README.md"))
	if m.ID() != expectedID {
		t.Errorf("创建出的 Note ID = %v, 想要 %v", m.ID(), expectedID)
	}
	if m.Content() != "测试内容" {
		t.Errorf("创建出的 Note Content = %q, 想要 %q", m.Content(), "测试内容")
	}

	// 2. 测试试图覆盖已存在的同名笔记，应返回 ALREADY_EXISTS 错误
	_, err = service.Create(ctx, dirRelPath, "README", "新内容")
	if err == nil {
		t.Fatal("尝试创建已存在的同名笔记，期望报错但返回了成功")
	}

	if apperror.ErrCode(err) != "ALREADY_EXISTS" {
		t.Errorf("期望的错误码是 ALREADY_EXISTS, 实际得到: %v", err)
	}
}

// 笔记名由用户输入，拼接结果重新围栏：越界的目标名无法被创建
func TestServiceCreateRejectsEscapingName(t *testing.T) {
	rootDir := testRootDir()
	repo := &mockRepo{notes: make(map[relpath.Path]*Note)}
	service := NewService(repo, NewFactory(rootDir), rootDir)

	_, err := service.Create(context.Background(), testutil.RelIn(t, rootDir, "subdir"), "../../escaped", "测试内容")
	if err == nil {
		t.Fatal("期望逃逸根目录的笔记名被拒绝，但创建成功了")
	}
}

func TestNewEmpty(t *testing.T) {
	rootDir := testRootDir()
	service := NewService(nil, NewFactory(rootDir), rootDir)
	relPath := testutil.RelIn(t, rootDir, "subdir/image.png.md")
	m := service.newEmpty(relPath)
	if m.ID() != encodeID(relPath) {
		t.Errorf("newEmpty().ID() = %v, 想要 %v", m.ID(), encodeID(relPath))
	}
	if m.RelPath() != relPath {
		t.Errorf("newEmpty().RelPath() = %q, 想要 %q", m.RelPath(), relPath)
	}
	expectedAbsPath := filepath.Join(rootDir, "subdir", "image.png.md")
	if m.AbsPath() != expectedAbsPath {
		t.Errorf("newEmpty().AbsPath() = %q, 想要 %q", m.AbsPath(), expectedAbsPath)
	}
	if m.Content() != "" {
		t.Errorf("newEmpty().Content() = %q, 想要 %q", m.Content(), "")
	}
}

func TestServiceReadNonExistent(t *testing.T) {
	rootDir := testRootDir()
	repo := &mockRepo{notes: make(map[relpath.Path]*Note)}
	service := NewService(repo, NewFactory(rootDir), rootDir)
	ctx := context.Background()

	relPath := testutil.RelIn(t, rootDir, "subdir/non_existent.md")
	id := encodeID(relPath)

	m, err := service.Read(ctx, id)
	if err != nil {
		t.Fatalf("Read 失败: %v", err)
	}
	if m == nil {
		t.Fatal("期望返回空 Note 实体，但返回了 nil")
	}
	if m.ID() != id {
		t.Errorf("期望 ID 为 %v, 实际为 %v", id, m.ID())
	}
	if m.Content() != "" {
		t.Errorf("期望内容为空，实际为 %q", m.Content())
	}
	expectedAbsPath := filepath.Join(rootDir, "subdir", "non_existent.md")
	if m.AbsPath() != expectedAbsPath {
		t.Errorf("期望绝对路径为 %q, 实际为 %q", expectedAbsPath, m.AbsPath())
	}
}

func TestServiceReadByRelPathNonExistent(t *testing.T) {
	rootDir := testRootDir()
	repo := &mockRepo{notes: make(map[relpath.Path]*Note)}
	service := NewService(repo, NewFactory(rootDir), rootDir)
	ctx := context.Background()

	relPath := testutil.RelIn(t, rootDir, "subdir/non_existent.md")
	m, err := service.ReadByRelPath(ctx, relPath)
	if err != nil {
		t.Fatalf("ReadByRelPath 失败: %v", err)
	}
	if m == nil {
		t.Fatal("期望返回空 Note 实体，但返回了 nil")
	}
	if m.RelPath() != relPath {
		t.Errorf("期望相对路径为 %q, 实际为 %q", relPath, m.RelPath())
	}
	expectedAbsPath := filepath.Join(rootDir, "subdir", "non_existent.md")
	if m.AbsPath() != expectedAbsPath {
		t.Errorf("期望绝对路径为 %q, 实际为 %q", expectedAbsPath, m.AbsPath())
	}
	if m.Content() != "" {
		t.Errorf("期望内容为空，实际为 %q", m.Content())
	}
}

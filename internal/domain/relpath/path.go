// Package relpath 提供「已确知位于根目录内」的根相对路径值对象。
//
// 构造即校验：能构造出来的 Path 必然位于 rootDir 之内，因此持有该类型本身就持有
// 「这条路径在根内」的保证，消费方无需（也不应）重复围栏。
//
// 该类型**不持有** rootDir（它被视为运行时配置而非领域值），因此 Abs / Join 这类
// 派生方法要求调用方传入 rootDir：类型无法校验传入的根与构造时的是否一致，
// 传入另一个根会得到「相对 A 根合法、相对 B 根越界」的结果。这是「值语义」取舍的代价，
// 调用方必须保证 rootDir 始终来自同一处配置注入。
package relpath

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Path 表示已确知位于根目录内的相对路径。
//
// 零值表示根目录本身，是安全值。内部统一为正斜杠分隔的规范形式，
// 构造时的 "."、空串、多余分隔符等都被规整掉。
//
// 该类型**不持有** rootDir：根目录是运行时配置而非路径自身的属性，
// 持有它会让值语义被单个物理目录污染。需要绝对路径时由调用方通过 Abs 传入。
type Path struct {
	// rel 以正斜杠分隔；根目录表示为空串
	rel string
}

// New 由根目录与根相对路径构造 Path，越界时直接返回错误。
//
// 这是字符串进入类型世界的唯一入口：构造成功即携带「位于根内」的保证，
// 构造失败即拒绝该路径，不存在「已构造但越界」的中间状态。
func New(rootDir, relPath string) (Path, error) {
	if err := ensureInRoot(rootDir, relPath); err != nil {
		return Path{}, err
	}

	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relPath)))
	if cleaned == "." {
		return Path{}, nil
	}
	return Path{rel: cleaned}, nil
}

// String 返回正斜杠形式的根相对路径；根目录返回空串。
func (p Path) String() string {
	return p.rel
}

// IsRoot 判断该路径是否指向根目录本身。
func (p Path) IsRoot() bool {
	return p.rel == ""
}

// Abs 返回该路径在 rootDir 下的物理绝对路径。
// 「根目录 + 根相对路径 = 绝对路径」这条规则只在此处实现。
func (p Path) Abs(rootDir string) string {
	if p.rel == "" {
		return filepath.Clean(rootDir)
	}
	return filepath.Join(rootDir, filepath.FromSlash(p.rel))
}

// Base 返回路径的最后一段名称；根目录返回空串。
func (p Path) Base() string {
	if p.rel == "" {
		return ""
	}
	return path.Base(p.rel)
}

// Dir 返回父目录路径；位于根目录时返回根目录自身。
// 去掉末段只会更靠近根，不可能逃逸，因此无需围栏。
func (p Path) Dir() Path {
	if p.rel == "" {
		return p
	}
	dir := path.Dir(p.rel)
	if dir == "." || dir == "/" {
		return Path{}
	}
	return Path{rel: dir}
}

// Join 在当前路径下拼接 name（name 可含分隔符），并对结果重新围栏。
func (p Path) Join(rootDir, name string) (Path, error) {
	return New(rootDir, filepath.Join(filepath.FromSlash(p.rel), filepath.FromSlash(name)))
}

// TrimExtSuffix 判断路径是否以 ext 结尾（大小写不敏感，如 ".xmp"）。
// 是则返回去掉该后缀的根相对路径并置 true；
// 否则**原样返回自身**并置 false —— 调用方几乎总是要用「处理后的路径」，
// 若不匹配时返回零值，非配套文件会被误当成根目录。
//
// 判定与裁剪必须同源：若调用方用大小写不敏感的规则判断、自己写大小写敏感的裁剪，
// `a.XMP` 这类文件名就会漏裁且无从察觉。
// 去掉后缀会改变路径语义（例如 `...xmp` 裁剪后即为 `..`），故重新围栏。
func (p Path) TrimExtSuffix(rootDir, ext string) (Path, bool, error) {
	if len(p.rel) < len(ext) || !strings.EqualFold(p.rel[len(p.rel)-len(ext):], ext) {
		return p, false, nil
	}
	trimmed, err := New(rootDir, p.rel[:len(p.rel)-len(ext)])
	if err != nil {
		return Path{}, false, err
	}
	return trimmed, true, nil
}

// ensureInRoot 判定 relPath 解析后是否仍位于 rootDir 之内。
func ensureInRoot(rootDir, relPath string) error {
	// 将输入的路径（通常是 Web/API 使用的正斜杠分隔路径）转换为当前操作系统的原生格式
	nativeRel := filepath.FromSlash(relPath)

	// 检查是否为绝对路径
	// 1. 使用 filepath.IsAbs (在 Windows 上需要有盘符或 UNC 路径才算绝对)
	// 2. 检查 Windows 卷名 (处理 C:foo 这种路径)
	// 3. 检查是否以路径分隔符开头 (处理 Windows 上的 \foo 驱动器相对路径，以及 Linux 上的 /foo)
	if filepath.IsAbs(nativeRel) || filepath.VolumeName(nativeRel) != "" || strings.HasPrefix(nativeRel, string(filepath.Separator)) {
		return fmt.Errorf("absolute path not allowed")
	}

	// 使用 filepath.Clean 清理路径，处理 . 和 ..
	cleanedRel := filepath.Clean(nativeRel)

	// 将清理后的相对路径拼接到根目录
	absPath := filepath.Join(rootDir, cleanedRel)

	// 使用 filepath.Rel 重新计算相对路径并验证
	// 这能有效防止通过 Join 产生的逃逸（例如在 Windows 上跨越驱动器盘符）
	rel, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// 最终验证：检查得到的相对路径是否尝试回到父目录
	// 我们需要检查它是否等于 ".." 或者以 ".." 分隔符开头
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes root directory")
	}

	return nil
}

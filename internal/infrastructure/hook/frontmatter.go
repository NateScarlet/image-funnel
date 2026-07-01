package hook

import (
	"fmt"
	"strings"
)

// parseFrontmatter 提取文件的 frontmatter 和 body。
// 如果没有 frontmatter，返回 "", content, false, newline
func parseFrontmatter(content string) (frontmatter, body string, has bool, newline string) {
	newline = "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}

	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", content, false, newline
	}

	parts := strings.SplitN(normalized, "---\n", 3)
	if len(parts) < 3 {
		return "", content, false, newline
	}

	// 统一用文件本身的换行符拼接
	frontmatter = strings.ReplaceAll(parts[1], "\n", newline)
	body = strings.ReplaceAll(parts[2], "\n", newline)
	return frontmatter, body, true, newline
}

func getHookRunID(content string) string {
	fm, _, has, newline := parseFrontmatter(content)
	if !has {
		return ""
	}
	lines := strings.SplitSeq(fm, newline)
	for line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		kv := strings.SplitN(trimmed, ":", 2)
		if len(kv) == 2 {
			k := strings.ToLower(strings.TrimSpace(kv[0]))
			if k == "hook-run-id" {
				return strings.TrimSpace(kv[1])
			}
		}
	}
	return ""
}

func setHookRunID(content string, runID string) string {
	fm, body, has, newline := parseFrontmatter(content)
	if !has {
		return fmt.Sprintf("---%[1]shook-run-id: %[2]s%[1]s---%[1]s%[3]s", newline, runID, content)
	}

	var sb strings.Builder
	var found bool
	for line := range strings.SplitSeq(fm, newline) {
		trimmed := strings.TrimSpace(line)
		if !found && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			kv := strings.SplitN(trimmed, ":", 2)
			if len(kv) == 2 {
				k := strings.ToLower(strings.TrimSpace(kv[0]))
				if k == "hook-run-id" {
					sb.WriteString("hook-run-id: ")
					sb.WriteString(runID)
					sb.WriteString(newline)
					found = true
					continue
				}
			}
		}
		sb.WriteString(line)
		sb.WriteString(newline)
	}

	if !found {
		// 如果未找到，我们直接在最前头插入
		return fmt.Sprintf("---%[1]shook-run-id: %[2]s%[1]s%[3]s---%[1]s%[4]s", newline, runID, sb.String(), body)
	}

	fmStr := strings.TrimSuffix(sb.String(), newline)
	return fmt.Sprintf("---%[1]s%[2]s%[1]s---%[1]s%[3]s", newline, fmStr, body)
}

func removeHookRunID(content string) string {
	fm, body, has, newline := parseFrontmatter(content)
	if !has {
		return content
	}

	var sb strings.Builder
	var hasActualLines bool
	for line := range strings.SplitSeq(fm, newline) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		kv := strings.SplitN(trimmed, ":", 2)
		if len(kv) == 2 {
			k := strings.ToLower(strings.TrimSpace(kv[0]))
			if k == "hook-run-id" {
				continue // 过滤移除 hook-run-id 这一行
			}
		}
		hasActualLines = true
		sb.WriteString(line)
		sb.WriteString(newline)
	}

	if !hasActualLines {
		// 剥离整个 frontmatter
		return body
	}

	fmStr := strings.TrimSuffix(sb.String(), newline)
	return fmt.Sprintf("---%[1]s%[2]s%[1]s---%[1]s%[3]s", newline, fmStr, body)
}
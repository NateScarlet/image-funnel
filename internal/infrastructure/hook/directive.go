package hook

import (
	"fmt"
	"strings"
	"time"
)

// applyDirectiveAction 根据指令动作（REMOVE/KEEP/COMMENT_OUT）返回替换后的文本行
// COMMENT_OUT 将指令行注释（%% ... %%），并追加 stdout/stderr 的 alert 语法块
// KEEP 保留原指令行，并追加 stdout/stderr 的 alert 语法块
func applyDirectiveAction(action string, matchedLine string, stdout string, stderr string, executedAt time.Time) string {
	if action == "REMOVE" {
		return ""
	}

	var newline = "\n"
	if strings.HasSuffix(matchedLine, "\r\n") {
		newline = "\r\n"
	}

	// 构建 alert 语法块（stdout + stderr）
	var alertBlock string
	ts := executedAt.Format("2006-01-02T15:04:05")
	stdoutTrimmed := strings.TrimRight(stdout, "\r\n")
	if stdoutTrimmed != "" {
		var sb strings.Builder
		fmt.Fprintf(&sb, ">[!stdout]%s%s", ts, newline)
		for line := range strings.SplitSeq(stdoutTrimmed, "\n") {
			line = strings.TrimSuffix(line, "\r")
			fmt.Fprintf(&sb, "> %s%s", line, newline)
		}
		alertBlock = sb.String()
	}
	stderrTrimmed := strings.TrimRight(stderr, "\r\n")
	if stderrTrimmed != "" {
		var sb strings.Builder
		sb.WriteString(alertBlock)
		fmt.Fprintf(&sb, ">[!stderr]%s%s", ts, newline)
		for line := range strings.SplitSeq(stderrTrimmed, "\n") {
			line = strings.TrimSuffix(line, "\r")
			fmt.Fprintf(&sb, "> %s%s", line, newline)
		}
		alertBlock = sb.String()
	}

	switch action {
	case "REMOVE":
		panic("should returned early")
	case "KEEP":
		if alertBlock != "" {
			return matchedLine + alertBlock
		}
		return matchedLine
	default: // COMMENT_OUT
		lineWithoutNL := strings.TrimSuffix(matchedLine, newline)
		trimmed := strings.TrimSpace(lineWithoutNL)
		commented := fmt.Sprintf("%%%% %s %%%%"+newline, trimmed)
		if alertBlock != "" {
			return commented + alertBlock
		}
		return commented
	}
}

// splitArgs 按空白分割参数字符串，支持双引号包裹含空格的参数
func splitArgs(s string) []string {
	var args []string
	var current strings.Builder
	inQuotes := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			inQuotes = !inQuotes
		} else if (ch == ' ' || ch == '\t') && !inQuotes {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}
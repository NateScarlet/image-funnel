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
			// 指令行若无尾换行（如位于笔记最后一行），需补换行再拼接 alert 块，
			// 否则 ">[!stdout]..." 会被粘到指令行末尾，下一次扫描时被误识别为指令参数
			if !strings.HasSuffix(matchedLine, "\n") {
				return matchedLine + newline + alertBlock
			}
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

// splitArgs 按空白分割参数字符串，支持单引号或双引号包裹含空格的参数，
// 引号内的内容原样保留，不支持转义。也用于将 hook 的 command 字段分词为 argv
func splitArgs(s string) []string {
	var args []string
	var current strings.Builder
	inSingleQuotes := false
	inDoubleQuotes := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '"' && !inSingleQuotes:
			inDoubleQuotes = !inDoubleQuotes
		case ch == '\'' && !inDoubleQuotes:
			inSingleQuotes = !inSingleQuotes
		case (ch == ' ' || ch == '\t') && !inSingleQuotes && !inDoubleQuotes:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}
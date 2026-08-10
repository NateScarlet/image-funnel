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

// splitArgs 按 shell 语法将字符串分割为参数：空白为分隔符，单引号或双引号包裹含空格的参数，
// 引号内内容原样保留，不支持转义。若引号未闭合则返回语法错误（快速失败，不静默吞字）。
func splitArgs(s string) ([]string, error) {
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
	if inSingleQuotes {
		return nil, fmt.Errorf("unterminated single quote in %q", s)
	}
	if inDoubleQuotes {
		return nil, fmt.Errorf("unterminated double quote in %q", s)
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args, nil
}

// parseCommandArgs 将 hook 的 command 字段分词为 argv，空命令或引号未闭合返回错误
func parseCommandArgs(command string) ([]string, error) {
	argv, err := splitArgs(command)
	if err != nil {
		return nil, err
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("command is empty: %q", command)
	}
	return argv, nil
}
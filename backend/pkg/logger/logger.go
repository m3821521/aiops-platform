package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New 创建结构化日志。
// 对应 Java 里的 SLF4J/Logback：先初始化，再全项目共用同一个 Logger。
func New(level string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lv = slog.LevelDebug
	case "warn", "warning":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lv})
	log := slog.New(handler)
	slog.SetDefault(log)
	return log
}

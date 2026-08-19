package logger_test

import (
	"github.com/aiops/aiops-platform/pkg/logger"
	"testing"
)

func TestNewLogger(t *testing.T) {
	log := logger.New("debug")
	if log == nil {
		t.Fatal("logger is nil")
	}
}

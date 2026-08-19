package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestID_Generated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		rid, exists := c.Get("request_id")
		if !exists {
			t.Error("request_id not set in context")
		}
		if rid == "" {
			t.Error("request_id is empty")
		}
		c.JSON(http.StatusOK, gin.H{"request_id": rid})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// 响应头应包含 X-Request-ID
	headerRID := w.Header().Get("X-Request-ID")
	if headerRID == "" {
		t.Error("X-Request-ID header is empty")
	}
}

func TestRequestID_Passthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		rid, _ := c.Get("request_id")
		c.JSON(http.StatusOK, gin.H{"request_id": rid})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "custom-trace-id-123")
	r.ServeHTTP(w, req)

	headerRID := w.Header().Get("X-Request-ID")
	if headerRID != "custom-trace-id-123" {
		t.Errorf("expected passthrough request_id, got %s", headerRID)
	}
}

func TestRequestID_Unique(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		rid, _ := c.Get("request_id")
		c.JSON(http.StatusOK, gin.H{"request_id": rid})
	})

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)
		rid := w.Header().Get("X-Request-ID")
		if ids[rid] {
			t.Errorf("duplicate request_id: %s", rid)
		}
		ids[rid] = true
	}
}

func TestGetRequestID_FromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		// 模拟下游 service 通过 context 获取 request_id
		rid := GetRequestID(c.Request.Context())
		if rid == "" {
			t.Error("GetRequestID returned empty")
		}
		c.JSON(http.StatusOK, gin.H{"request_id": rid})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

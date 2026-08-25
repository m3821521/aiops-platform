package response

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestProvenanceSerialization(t *testing.T) {
	now := time.Now().UTC()
	prov := &Provenance{
		Source:              "prometheus",
		SourceType:          "provider",
		FetchedAt:           &now,
		DataTimestamp:       &now,
		TimestampAvailable:  true,
		TimestampSemantics:  "latest_prometheus_sample_timestamp",
		CacheHit:            false,
	}

	body := Body{
		Code:    0,
		Message: "success",
		Data:    map[string]interface{}{"result": "ok"},
		Meta:    &Meta{Provenance: prov},
	}

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded Body
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Meta == nil || decoded.Meta.Provenance == nil {
		t.Fatal("meta.provenance should not be nil")
	}
	if decoded.Meta.Provenance.Source != "prometheus" {
		t.Errorf("source = %s, want prometheus", decoded.Meta.Provenance.Source)
	}
	if decoded.Meta.Provenance.TimestampAvailable != true {
		t.Error("timestampAvailable should be true")
	}
	if decoded.Meta.Provenance.CacheHit != false {
		t.Error("cacheHit should be false")
	}
}

func TestProvenanceNilTimestamps(t *testing.T) {
	// 无法获得 dataTimestamp 时，DataTimestamp 应为 nil，TimestampAvailable=false
	prov := &Provenance{
		Source:             "kubernetes",
		SourceType:         "provider",
		TimestampAvailable: false,
		CacheHit:           false,
	}

	data, err := json.Marshal(prov)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// 验证 dataTimestamp 字段不出现在 JSON 中（omitempty）
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if _, exists := m["dataTimestamp"]; exists {
		t.Error("dataTimestamp should be omitted when nil")
	}
	if _, exists := m["fetchedAt"]; exists {
		t.Error("fetchedAt should be omitted when nil")
	}
}

func TestOKWithProvenance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	now := time.Now().UTC()
	prov := &Provenance{
		Source:             "topology",
		SourceType:         "redis-cache",
		FetchedAt:          &now,
		CacheHit:           true,
		TimestampAvailable: false,
	}

	OKWithProvenance(c, map[string]interface{}{"nodes": 10}, prov)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var body Body
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if body.Code != 0 {
		t.Errorf("code = %d, want 0", body.Code)
	}
	if body.Meta == nil || body.Meta.Provenance == nil {
		t.Fatal("meta.provenance should not be nil")
	}
	if body.Meta.Provenance.CacheHit != true {
		t.Error("cacheHit should be true")
	}
	if body.Meta.Provenance.Source != "topology" {
		t.Errorf("source = %s, want topology", body.Meta.Provenance.Source)
	}
}

func TestOKBackwardCompatible(t *testing.T) {
	// 验证旧的 OK(c, data) 不包含 meta 字段
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	OK(c, map[string]interface{}{"items": 5})

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if _, exists := body["meta"]; exists {
		t.Error("meta should not be present in backward-compatible OK()")
	}
}

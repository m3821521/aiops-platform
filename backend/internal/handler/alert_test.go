package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/aiops/aiops-platform/internal/alert"
	"github.com/aiops/aiops-platform/internal/api"
	"github.com/aiops/aiops-platform/internal/auth"
	"github.com/aiops/aiops-platform/internal/cluster"
	"github.com/aiops/aiops-platform/internal/handler"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestDB 创建一个 SQLite 内存数据库，用于测试告警存储层。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&alert.Alert{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// newAuthService 创建一个测试用的 AuthService，并返回一个有效的 admin token。
func newAuthService(t *testing.T, db *gorm.DB) (*auth.Service, string) {
	t.Helper()
	if err := db.AutoMigrate(&auth.User{}, &auth.Role{}, &auth.Permission{}); err != nil {
		t.Fatal(err)
	}
	authRepo := auth.NewRepository(db)
	authSvc := auth.NewService(authRepo, "test-secret-key-for-testing-only", time.Hour)

	// 创建 admin 角色和用户
	adminRole := &auth.Role{Name: "admin", Description: "admin"}
	if err := db.Create(adminRole).Error; err != nil {
		t.Fatal(err)
	}
	hashedPassword, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	adminUser := &auth.User{
		Username:     "admin",
		PasswordHash: hashedPassword,
		Status:       "active",
		Roles:        []auth.Role{*adminRole},
	}
	if err := db.Create(adminUser).Error; err != nil {
		t.Fatal(err)
	}

	// 生成 token
	loginResp, err := authSvc.Login(context.Background(), auth.LoginRequest{
		Username: "admin",
		Password: "test-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	return authSvc, loginResp.AccessToken
}

func newAlertRouter(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	return newAlertRouterWithAuth(t, db, nil)
}

func newAlertRouterWithAuth(t *testing.T, db *gorm.DB, authSvc *auth.Service) *gin.Engine {
	t.Helper()
	repo := alert.NewRepository(db)
	deps := api.Deps{
		Health:  &handler.HealthHandler{},
		Cluster: &handler.ClusterHandler{Service: cluster.NewService(cluster.NewManager(nil))},
		Alert: &handler.AlertHandler{
			Repo:         repo,
			Aggregator:   alert.NewAggregator(repo),
			NoiseReducer: alert.NewNoiseReducer(repo),
		},
	}
	if authSvc != nil {
		deps.AuthService = authSvc
	}
	return api.NewRouter("test", deps)
}

func TestAlertWebhookReceive(t *testing.T) {
	db := newTestDB(t)
	r := newAlertRouter(t, db)

	payload := `{
		"receiver": "aiops",
		"status": "firing",
		"alerts": [
			{
				"status": "firing",
				"labels": {
					"alertname": "HighCPU",
					"severity": "critical",
					"instance": "node1",
					"namespace": "default",
					"pod": "order-service-abc",
					"service": "order-service"
				},
				"annotations": {"summary": "CPU > 90%"},
				"startsAt": "2026-01-01T00:00:00Z",
				"fingerprint": "fp-001"
			}
		]
	}`

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/webhook", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	// 验证数据库中有一条记录。
	var count int64
	db.Model(&alert.Alert{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 alert, got %d", count)
	}

	var a alert.Alert
	db.First(&a)
	if a.Alertname != "HighCPU" || a.Severity != "critical" || a.Status != "firing" {
		t.Fatalf("unexpected alert: %+v", a)
	}
}

func TestAlertWebhookUpsert(t *testing.T) {
	db := newTestDB(t)
	r := newAlertRouter(t, db)

	payload := `{
		"status": "firing",
		"alerts": [
			{"status": "firing", "labels": {"alertname": "TestAlert", "severity": "warning"},
			 "startsAt": "2026-01-01T00:00:00Z", "fingerprint": "fp-upsert"}
		]
	}`

	// 第一次推送。
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/webhook", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// 第二次推送相同 fingerprint，应更新而非创建。
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/alerts/webhook", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	var count int64
	db.Model(&alert.Alert{}).Count(&count)
	if count != 1 {
		t.Fatalf("upsert should not create duplicate, expected 1, got %d", count)
	}
}

func TestAlertList(t *testing.T) {
	db := newTestDB(t)
	repo := alert.NewRepository(db)

	// 插入两条测试数据。
	repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "list-1", Alertname: "AlertA", Severity: "critical",
		Status: "firing", StartsAt: time.Now(),
	})
	repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "list-2", Alertname: "AlertB", Severity: "warning",
		Status: "resolved", StartsAt: time.Now().Add(-time.Hour),
	})

	r := newAlertRouter(t, db)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts?page=1&page_size=10", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Items []alert.Alert `json:"items"`
			Total int64         `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Total != 2 {
		t.Fatalf("expected total=2, got %d", resp.Data.Total)
	}
	if len(resp.Data.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Data.Items))
	}
}

func TestAlertListFilterByStatus(t *testing.T) {
	db := newTestDB(t)
	repo := alert.NewRepository(db)
	repo.Upsert(t.Context(), &alert.Alert{Fingerprint: "f1", Alertname: "A", Status: "firing", StartsAt: time.Now()})
	repo.Upsert(t.Context(), &alert.Alert{Fingerprint: "f2", Alertname: "B", Status: "resolved", StartsAt: time.Now()})

	r := newAlertRouter(t, db)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts?status=firing", nil)
	r.ServeHTTP(w, req)

	var resp struct {
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Total != 1 {
		t.Fatalf("expected 1 firing alert, got %d", resp.Data.Total)
	}
}

func TestAlertGet(t *testing.T) {
	db := newTestDB(t)
	repo := alert.NewRepository(db)
	saved, _ := repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "get-1", Alertname: "GetTest", Severity: "warning",
		Status: "firing", StartsAt: time.Now(),
	})

	r := newAlertRouter(t, db)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/"+itoa(saved.ID), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAlertGetNotFound(t *testing.T) {
	db := newTestDB(t)
	r := newAlertRouter(t, db)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/99999", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAlertAcknowledge(t *testing.T) {
	db := newTestDB(t)
	repo := alert.NewRepository(db)
	saved, _ := repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "ack-1", Alertname: "AckTest", Status: "firing", StartsAt: time.Now(),
	})

	authSvc, token := newAuthService(t, db)
	r := newAlertRouterWithAuth(t, db, authSvc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/"+itoa(saved.ID)+"/acknowledge", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	a, _ := repo.FindByID(t.Context(), saved.ID)
	if a.Status != "acknowledged" {
		t.Fatalf("expected status=acknowledged, got %s", a.Status)
	}
}

func TestAlertResolve(t *testing.T) {
	db := newTestDB(t)
	repo := alert.NewRepository(db)
	saved, _ := repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "res-1", Alertname: "ResTest", Status: "firing", StartsAt: time.Now(),
	})

	authSvc, token := newAuthService(t, db)
	r := newAlertRouterWithAuth(t, db, authSvc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/"+itoa(saved.ID)+"/resolve", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	a, _ := repo.FindByID(t.Context(), saved.ID)
	if a.Status != "resolved" {
		t.Fatalf("expected status=resolved, got %s", a.Status)
	}
	if a.EndsAt == nil {
		t.Fatal("expected ends_at to be set")
	}
}

func TestWebhookAlertToAlert(t *testing.T) {
	wa := alert.WebhookAlert{
		Status: "firing",
		Labels: map[string]string{
			"alertname": "TestAlert",
			"severity":  "critical",
			"instance":  "node1",
			"pod":       "pod1",
			"namespace": "default",
			"service":   "svc1",
			"node":      "node1",
		},
		Annotations: map[string]string{"summary": "test"},
		StartsAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Fingerprint: "fp-test",
	}

	a := wa.ToAlert()
	if a.Alertname != "TestAlert" {
		t.Fatalf("alertname=%s", a.Alertname)
	}
	if a.Severity != "critical" {
		t.Fatalf("severity=%s", a.Severity)
	}
	if a.Pod != "pod1" || a.Namespace != "default" || a.Service != "svc1" {
		t.Fatalf("unexpected fields: %+v", a)
	}
	if a.EndsAt != nil {
		t.Fatal("firing alert should not have ends_at")
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func TestAlertAggregateByService(t *testing.T) {
	db := newTestDB(t)
	repo := alert.NewRepository(db)

	// 同一个服务的两条告警，应该聚合为一组。
	repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "agg-1", Alertname: "HighCPU", Severity: "warning",
		Status: "firing", Namespace: "default", Service: "order-service", StartsAt: time.Now(),
	})
	repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "agg-2", Alertname: "HighMemory", Severity: "critical",
		Status: "firing", Namespace: "default", Service: "order-service", StartsAt: time.Now(),
	})
	// 另一个服务的告警。
	repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "agg-3", Alertname: "PodDown", Severity: "critical",
		Status: "firing", Namespace: "default", Service: "payment-service", StartsAt: time.Now(),
	})

	r := newAlertRouter(t, db)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/aggregate?dimension=service", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Code int                `json:"code"`
		Data []alert.AlertGroup `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(resp.Data))
	}

	// order-service 组应该有 2 条告警，最高级别 critical。
	var orderGroup *alert.AlertGroup
	for i := range resp.Data {
		if resp.Data[i].Key == "default/order-service" {
			orderGroup = &resp.Data[i]
		}
	}
	if orderGroup == nil {
		t.Fatal("order-service group not found")
	}
	if orderGroup.Count != 2 {
		t.Fatalf("expected count=2, got %d", orderGroup.Count)
	}
	if orderGroup.Severity != "critical" {
		t.Fatalf("expected severity=critical, got %s", orderGroup.Severity)
	}
}

func TestAlertAggregateByNode(t *testing.T) {
	db := newTestDB(t)
	repo := alert.NewRepository(db)

	repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "node-1", Alertname: "NodeCPU", Severity: "warning",
		Status: "firing", Node: "worker-1", StartsAt: time.Now(),
	})
	repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "node-2", Alertname: "NodeDisk", Severity: "info",
		Status: "firing", Node: "worker-1", StartsAt: time.Now(),
	})

	r := newAlertRouter(t, db)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/aggregate?dimension=node", nil)
	r.ServeHTTP(w, req)

	var resp struct {
		Data []alert.AlertGroup `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 group, got %d", len(resp.Data))
	}
	if resp.Data[0].Key != "worker-1" {
		t.Fatalf("expected key=worker-1, got %s", resp.Data[0].Key)
	}
}

func TestAlertAggregateInvalidDimension(t *testing.T) {
	db := newTestDB(t)
	r := newAlertRouter(t, db)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/aggregate?dimension=invalid", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAlertAggregateEmpty(t *testing.T) {
	db := newTestDB(t)
	r := newAlertRouter(t, db)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/aggregate", nil)
	r.ServeHTTP(w, req)

	var resp struct {
		Data []alert.AlertGroup `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(resp.Data))
	}
}

func TestAlertNoiseDedup(t *testing.T) {
	db := newTestDB(t)
	repo := alert.NewRepository(db)

	// 插入两条相同 fingerprint 的告警（模拟重复推送），Upsert 会更新而非创建，
	// 所以这里用不同 fingerprint 但相同服务来测试分组去重。
	now := time.Now()
	repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "noise-1", Alertname: "HighCPU", Severity: "warning",
		Status: "firing", Namespace: "default", Service: "svc-a", StartsAt: now,
	})
	repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "noise-2", Alertname: "HighMemory", Severity: "critical",
		Status: "firing", Namespace: "default", Service: "svc-a", StartsAt: now,
	})

	r := newAlertRouter(t, db)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/noise?dimension=service", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data alert.NoiseResult `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.TotalBefore != 2 {
		t.Fatalf("expected total_before=2, got %d", resp.Data.TotalBefore)
	}
	if resp.Data.TotalAfter != 2 {
		t.Fatalf("expected total_after=2, got %d", resp.Data.TotalAfter)
	}
	if len(resp.Data.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(resp.Data.Groups))
	}
}

func TestAlertNoiseWindowFilter(t *testing.T) {
	db := newTestDB(t)
	repo := alert.NewRepository(db)

	// 一条窗口内的告警。
	repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "win-1", Alertname: "Recent", Severity: "warning",
		Status: "firing", Service: "svc-a", StartsAt: time.Now(),
	})
	// 一条窗口外的告警（1 小时前）。
	repo.Upsert(t.Context(), &alert.Alert{
		Fingerprint: "win-2", Alertname: "Old", Severity: "warning",
		Status: "firing", Service: "svc-b", StartsAt: time.Now().Add(-1 * time.Hour),
	})

	r := newAlertRouter(t, db)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/noise?dimension=service", nil)
	r.ServeHTTP(w, req)

	var resp struct {
		Data alert.NoiseResult `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// total_before 包含所有 firing 告警（2 条），total_after 只包含窗口内的（1 条）。
	if resp.Data.TotalBefore != 2 {
		t.Fatalf("expected total_before=2, got %d", resp.Data.TotalBefore)
	}
	if resp.Data.TotalAfter != 1 {
		t.Fatalf("expected total_after=1 (window filter), got %d", resp.Data.TotalAfter)
	}
}

func TestAlertNoiseStormDetection(t *testing.T) {
	db := newTestDB(t)
	repo := alert.NewRepository(db)

	// 用自定义配置：风暴阈值设为 3，插入 5 条同服务告警。
	for i := 0; i < 5; i++ {
		repo.Upsert(t.Context(), &alert.Alert{
			Fingerprint: "storm-" + itoa(int64(i)),
			Alertname:   "Alert" + itoa(int64(i)),
			Severity:    "warning",
			Status:      "firing",
			Service:     "storm-svc",
			StartsAt:    time.Now(),
		})
	}

	// 直接用自定义配置的 NoiseReducer 测试。
	reducer := alert.NewNoiseReducerWithConfig(repo, alert.NoiseConfig{
		Window:         5 * time.Minute,
		StormThreshold: 3,
		TotalStorm:     100,
	})
	result, err := reducer.Reduce(t.Context(), "service")
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsStorm {
		t.Fatal("expected storm detection")
	}
	if result.StormReason == "" {
		t.Fatal("expected storm reason")
	}
}

func TestAlertNoiseEmpty(t *testing.T) {
	db := newTestDB(t)
	r := newAlertRouter(t, db)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/noise", nil)
	r.ServeHTTP(w, req)

	var resp struct {
		Data alert.NoiseResult `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.TotalBefore != 0 {
		t.Fatalf("expected 0, got %d", resp.Data.TotalBefore)
	}
	if resp.Data.IsStorm {
		t.Fatal("should not be storm with 0 alerts")
	}
}

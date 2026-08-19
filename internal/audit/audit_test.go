package audit_test

import (
	"context"
	"testing"

	"github.com/aiops/aiops-platform/internal/audit"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAuditDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&audit.Log{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestAuditCreateAndList(t *testing.T) {
	db := setupAuditDB(t)
	repo := audit.NewRepository(db)

	log := &audit.Log{
		Username:   "admin",
		Action:     "POST",
		Resource:   "alerts",
		ResourceID: "123",
		IP:         "127.0.0.1",
		Result:     "success",
	}
	if err := repo.Create(context.Background(), log); err != nil {
		t.Fatal(err)
	}
	if log.ID == 0 {
		t.Fatal("expected ID to be set")
	}

	logs, total, err := repo.List(context.Background(), audit.ListFilter{}, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Username != "admin" {
		t.Fatalf("expected username admin, got %s", logs[0].Username)
	}
}

func TestAuditFilterByAction(t *testing.T) {
	db := setupAuditDB(t)
	repo := audit.NewRepository(db)

	repo.Create(context.Background(), &audit.Log{Action: "POST", Resource: "alerts", Result: "success"})
	repo.Create(context.Background(), &audit.Log{Action: "GET", Resource: "alerts", Result: "success"})
	repo.Create(context.Background(), &audit.Log{Action: "DELETE", Resource: "pods", Result: "success"})

	logs, total, err := repo.List(context.Background(), audit.ListFilter{Action: "POST"}, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("expected total 1 for POST, got %d", total)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
}

func TestAuditFilterByResource(t *testing.T) {
	db := setupAuditDB(t)
	repo := audit.NewRepository(db)

	repo.Create(context.Background(), &audit.Log{Action: "POST", Resource: "alerts", Result: "success"})
	repo.Create(context.Background(), &audit.Log{Action: "POST", Resource: "pods", Result: "success"})

	logs, total, err := repo.List(context.Background(), audit.ListFilter{Resource: "pods"}, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("expected total 1 for pods, got %d", total)
	}
	if logs[0].Resource != "pods" {
		t.Fatalf("expected resource pods, got %s", logs[0].Resource)
	}
}

func TestAuditFilterByResult(t *testing.T) {
	db := setupAuditDB(t)
	repo := audit.NewRepository(db)

	repo.Create(context.Background(), &audit.Log{Action: "POST", Resource: "alerts", Result: "success"})
	repo.Create(context.Background(), &audit.Log{Action: "POST", Resource: "alerts", Result: "failed"})

	logs, total, err := repo.List(context.Background(), audit.ListFilter{Result: "failed"}, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("expected total 1 for failed, got %d", total)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
}

func TestAuditPagination(t *testing.T) {
	db := setupAuditDB(t)
	repo := audit.NewRepository(db)

	for i := 0; i < 25; i++ {
		repo.Create(context.Background(), &audit.Log{Action: "POST", Resource: "test", Result: "success"})
	}

	// 第一页。
	logs, total, err := repo.List(context.Background(), audit.ListFilter{}, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 25 {
		t.Fatalf("expected total 25, got %d", total)
	}
	if len(logs) != 10 {
		t.Fatalf("expected 10 logs on page 1, got %d", len(logs))
	}

	// 第三页应该只有 5 条。
	page3Logs, _, err := repo.List(context.Background(), audit.ListFilter{}, 3, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page3Logs) != 5 {
		t.Fatalf("expected 5 logs on page 3, got %d", len(page3Logs))
	}
}

func TestAuditLogTableName(t *testing.T) {
	log := audit.Log{}
	if log.TableName() != "audit_logs" {
		t.Fatalf("expected table name audit_logs, got %s", log.TableName())
	}
}

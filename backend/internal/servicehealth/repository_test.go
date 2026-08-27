package servicehealth

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB 创建内存 SQLite 数据库并迁移 Service 表。
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&Service{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestRepositoryCreate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	svc := &Service{
		Name:         "api",
		Namespace:    "default",
		Cluster:      "prod",
		WorkloadType: WorkloadTypeDeployment,
		WorkloadName: "api",
		ServiceType:  "ClusterIP",
	}

	created, err := repo.Create(context.Background(), svc)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if created.Name != "api" {
		t.Errorf("expected Name=api, got %s", created.Name)
	}
}

func TestRepositoryFindByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	svc, _ := repo.Create(context.Background(), &Service{
		Name: "api", Namespace: "default", Cluster: "prod",
	})

	found, err := repo.FindByID(context.Background(), svc.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected non-nil result")
	}
	if found.Name != "api" {
		t.Errorf("expected Name=api, got %s", found.Name)
	}

	// 不存在的 ID
	notFound, err := repo.FindByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf("FindByID non-existent failed: %v", err)
	}
	if notFound != nil {
		t.Error("expected nil for non-existent ID")
	}
}

func TestRepositoryFindByIdentity(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	_, _ = repo.Create(context.Background(), &Service{
		Name: "api", Namespace: "default", Cluster: "prod",
	})

	found, err := repo.FindByIdentity(context.Background(), "prod", "default", "api")
	if err != nil {
		t.Fatalf("FindByIdentity failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected non-nil result")
	}

	// 不存在的 identity
	notFound, err := repo.FindByIdentity(context.Background(), "prod", "default", "nonexistent")
	if err != nil {
		t.Fatalf("FindByIdentity non-existent failed: %v", err)
	}
	if notFound != nil {
		t.Error("expected nil for non-existent identity")
	}
}

func TestRepositoryList(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	for i := 0; i < 5; i++ {
		_, _ = repo.Create(context.Background(), &Service{
			Name: "svc-" + string(rune('a'+i)), Namespace: "default", Cluster: "prod",
		})
	}

	items, total, err := repo.List(context.Background(), ListFilter{Cluster: "prod"}, 1, 10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total=5, got %d", total)
	}
	if len(items) != 5 {
		t.Errorf("expected 5 items, got %d", len(items))
	}
}

func TestRepositoryListClusterFilter(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	_, _ = repo.Create(context.Background(), &Service{Name: "api", Namespace: "default", Cluster: "prod"})
	_, _ = repo.Create(context.Background(), &Service{Name: "api", Namespace: "default", Cluster: "staging"})

	items, total, err := repo.List(context.Background(), ListFilter{Cluster: "prod"}, 1, 10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total=1 for prod, got %d", total)
	}
	if items[0].Cluster != "prod" {
		t.Errorf("expected cluster=prod, got %s", items[0].Cluster)
	}
}

func TestRepositoryListNamespaceFilter(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	_, _ = repo.Create(context.Background(), &Service{Name: "api", Namespace: "default", Cluster: "prod"})
	_, _ = repo.Create(context.Background(), &Service{Name: "api", Namespace: "monitoring", Cluster: "prod"})

	items, total, err := repo.List(context.Background(), ListFilter{Cluster: "prod", Namespace: "monitoring"}, 1, 10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total=1 for monitoring, got %d", total)
	}
	if items[0].Namespace != "monitoring" {
		t.Errorf("expected namespace=monitoring, got %s", items[0].Namespace)
	}
}

func TestRepositoryListByCluster(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	_, _ = repo.Create(context.Background(), &Service{Name: "api", Namespace: "default", Cluster: "prod"})
	_, _ = repo.Create(context.Background(), &Service{Name: "web", Namespace: "default", Cluster: "prod"})
	_, _ = repo.Create(context.Background(), &Service{Name: "api", Namespace: "default", Cluster: "staging"})

	items, err := repo.ListByCluster(context.Background(), "prod")
	if err != nil {
		t.Fatalf("ListByCluster failed: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items for prod, got %d", len(items))
	}
}

func TestRepositoryListByNamespace(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	_, _ = repo.Create(context.Background(), &Service{Name: "api", Namespace: "default", Cluster: "prod"})
	_, _ = repo.Create(context.Background(), &Service{Name: "web", Namespace: "monitoring", Cluster: "prod"})

	items, err := repo.ListByNamespace(context.Background(), "prod", "default")
	if err != nil {
		t.Fatalf("ListByNamespace failed: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item for prod/default, got %d", len(items))
	}
}

func TestRepositoryUpsert(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// 第一次插入
	svc1 := &Service{
		Name: "api", Namespace: "default", Cluster: "prod",
		WorkloadType: WorkloadTypeDeployment, WorkloadName: "api-v1",
	}
	result1, err := repo.Upsert(context.Background(), svc1)
	if err != nil {
		t.Fatalf("Upsert create failed: %v", err)
	}
	if result1.WorkloadName != "api-v1" {
		t.Errorf("expected WorkloadName=api-v1, got %s", result1.WorkloadName)
	}

	// 第二次更新（相同 identity）
	svc2 := &Service{
		Name: "api", Namespace: "default", Cluster: "prod",
		WorkloadType: WorkloadTypeDeployment, WorkloadName: "api-v2",
	}
	result2, err := repo.Upsert(context.Background(), svc2)
	if err != nil {
		t.Fatalf("Upsert update failed: %v", err)
	}
	if result2.ID != result1.ID {
		t.Errorf("expected same ID after upsert, got %d vs %d", result2.ID, result1.ID)
	}
	if result2.WorkloadName != "api-v2" {
		t.Errorf("expected WorkloadName=api-v2 after update, got %s", result2.WorkloadName)
	}

	// 验证只有一条记录
	items, total, _ := repo.List(context.Background(), ListFilter{Cluster: "prod"}, 1, 10)
	if total != 1 {
		t.Errorf("expected total=1 after upsert, got %d", total)
	}
	_ = items
}

func TestRepositoryUpsertMissingIdentity(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	_, err := repo.Upsert(context.Background(), &Service{Name: "api"}) // 缺 cluster/namespace
	if err == nil {
		t.Error("expected error for missing identity")
	}
}

func TestRepositoryUpdate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	svc, _ := repo.Create(context.Background(), &Service{
		Name: "api", Namespace: "default", Cluster: "prod",
		WorkloadName: "api-v1",
	})

	svc.WorkloadName = "api-v2"
	if err := repo.Update(context.Background(), svc); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	found, _ := repo.FindByID(context.Background(), svc.ID)
	if found.WorkloadName != "api-v2" {
		t.Errorf("expected WorkloadName=api-v2 after update, got %s", found.WorkloadName)
	}
}

func TestRepositoryIdentityUniqueness(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// 相同 identity 直接 Create 两次应该失败（唯一约束）
	_, _ = repo.Create(context.Background(), &Service{
		Name: "api", Namespace: "default", Cluster: "prod",
	})

	_, err := repo.Create(context.Background(), &Service{
		Name: "api", Namespace: "default", Cluster: "prod",
	})
	if err == nil {
		t.Error("expected unique constraint violation for duplicate identity")
	}
}

func TestRepositoryDifferentClusterSameName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// 不同 cluster 同名 Service 应该可以共存
	_, err1 := repo.Create(context.Background(), &Service{Name: "api", Namespace: "default", Cluster: "prod"})
	_, err2 := repo.Create(context.Background(), &Service{Name: "api", Namespace: "default", Cluster: "staging"})

	if err1 != nil || err2 != nil {
		t.Errorf("expected both creates to succeed, got err1=%v, err2=%v", err1, err2)
	}

	items, total, _ := repo.List(context.Background(), ListFilter{}, 1, 10)
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
	_ = items
}

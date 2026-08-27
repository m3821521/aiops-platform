package servicehealth

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 提供 Service Model 的持久化操作。
// 所有查询必须考虑 Cluster 条件，Namespace 非空时必须过滤 Namespace，
// 防止跨集群 / 跨命名空间数据污染。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建 Service Repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ListFilter 是 Service 列表的过滤条件。
type ListFilter struct {
	Cluster   string
	Namespace string
}

// Create 创建新 Service。
func (r *Repository) Create(ctx context.Context, svc *Service) (*Service, error) {
	if err := r.db.WithContext(ctx).Create(svc).Error; err != nil {
		return nil, err
	}
	return svc, nil
}

// FindByID 按主键查询 Service。
func (r *Repository) FindByID(ctx context.Context, id int64) (*Service, error) {
	var svc Service
	if err := r.db.WithContext(ctx).First(&svc, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &svc, nil
}

// FindByIDAndCluster 按主键 + cluster 查询 Service。
// P2-01 Phase 3 G5: Service ID 不能单独决定访问权限，必须校验 cluster。
// cluster 不匹配时返回 nil（与 not found 相同语义），防止跨集群越权访问。
func (r *Repository) FindByIDAndCluster(ctx context.Context, id int64, cluster string) (*Service, error) {
	if cluster == "" {
		return nil, errors.New("cluster is required for FindByIDAndCluster")
	}
	var svc Service
	err := r.db.WithContext(ctx).
		Where("id = ? AND cluster = ?", id, cluster).
		First(&svc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &svc, nil
}

// FindByIdentity 按业务唯一键 (cluster + namespace + name) 查询 Service。
func (r *Repository) FindByIdentity(ctx context.Context, cluster, namespace, name string) (*Service, error) {
	var svc Service
	err := r.db.WithContext(ctx).
		Where("cluster = ? AND namespace = ? AND name = ?", cluster, namespace, name).
		First(&svc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &svc, nil
}

// List 分页查询 Service，按 cluster + namespace + name 排序。
func (r *Repository) List(ctx context.Context, filter ListFilter, page, pageSize int) ([]Service, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	q := r.db.WithContext(ctx).Model(&Service{})
	q = applyFilter(q, filter)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []Service
	if err := q.
		Order("cluster ASC, namespace ASC, name ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListByCluster 查询指定集群下的所有 Service（不分页）。
func (r *Repository) ListByCluster(ctx context.Context, cluster string) ([]Service, error) {
	var items []Service
	if err := r.db.WithContext(ctx).
		Where("cluster = ?", cluster).
		Order("namespace ASC, name ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// ListByNamespace 查询指定集群 + 命名空间下的所有 Service（不分页）。
func (r *Repository) ListByNamespace(ctx context.Context, cluster, namespace string) ([]Service, error) {
	var items []Service
	if err := r.db.WithContext(ctx).
		Where("cluster = ? AND namespace = ?", cluster, namespace).
		Order("name ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// Upsert 按业务唯一键 (cluster + namespace + name) 执行插入或更新。
// 已存在则更新非 identity 字段，不存在则创建。
func (r *Repository) Upsert(ctx context.Context, svc *Service) (*Service, error) {
	if svc.Cluster == "" || svc.Namespace == "" || svc.Name == "" {
		return nil, errors.New("service identity (cluster/namespace/name) required")
	}

	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "cluster"},
				{Name: "namespace"},
				{Name: "name"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"workload_type",
				"workload_name",
				"workload_selector",
				"service_type",
				"owner",
				"updated_at",
			}),
		}).
		Create(svc).Error
	if err != nil {
		return nil, err
	}

	// OnConflict 后 GORM 不会回填已存在记录的 ID，重新查询以确保返回完整记录。
	return r.FindByIdentity(ctx, svc.Cluster, svc.Namespace, svc.Name)
}

// Update 更新 Service 字段（按主键）。
func (r *Repository) Update(ctx context.Context, svc *Service) error {
	if svc.ID == 0 {
		return errors.New("service id required")
	}
	return r.db.WithContext(ctx).Save(svc).Error
}

// applyFilter 将 ListFilter 条件应用到 GORM 查询。
func applyFilter(q *gorm.DB, filter ListFilter) *gorm.DB {
	if filter.Cluster != "" {
		q = q.Where("cluster = ?", filter.Cluster)
	}
	if filter.Namespace != "" {
		q = q.Where("namespace = ?", filter.Namespace)
	}
	return q
}

package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/aiops/aiops-platform/internal/alert"
	"github.com/aiops/aiops-platform/internal/anomaly"
	"github.com/aiops/aiops-platform/internal/cluster"
	"github.com/aiops/aiops-platform/internal/incident"
	"github.com/aiops/aiops-platform/internal/logging"
	"github.com/aiops/aiops-platform/internal/monitoring"
	"github.com/aiops/aiops-platform/internal/rca"
	"github.com/aiops/aiops-platform/internal/topology"
)

// ===== get_incident =====

type GetIncidentTool struct {
	repo *incident.Repository
}

func NewGetIncidentTool(repo *incident.Repository) *GetIncidentTool {
	return &GetIncidentTool{repo: repo}
}
func (t *GetIncidentTool) Name() string        { return "get_incident" }
func (t *GetIncidentTool) Description() string { return "获取指定 Incident 的详细信息，包括状态、严重程度、关联资源、信号列表和时间线" }
func (t *GetIncidentTool) ReadOnly() bool      { return true }
func (t *GetIncidentTool) InputSchema() ToolSchema {
	return ToolSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"incident_id": {Type: "integer", Description: "Incident ID"},
		},
		Required: []string{"incident_id"},
	}
}
func (t *GetIncidentTool) Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error) {
	id := int64(input["incident_id"].(float64))
	inc, err := t.repo.FindByID(ctx, id)
	if err != nil {
		return ToolResult{ToolName: t.Name(), Success: false, Error: err.Error(), Source: "mysql", Timestamp: time.Now()}, nil
	}
	signals, _ := t.repo.ListSignals(ctx, id)
	return ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Available: true,
		Data:      map[string]interface{}{"incident": inc, "signals": signals},
		Source:    "mysql",
		Timestamp: time.Now(),
	}, nil
}

// ===== get_rca =====

type GetRCATool struct {
	svc *rca.Service
}

func NewGetRCATool(svc *rca.Service) *GetRCATool { return &GetRCATool{svc: svc} }
func (t *GetRCATool) Name() string               { return "get_rca" }
func (t *GetRCATool) Description() string        { return "获取指定 Incident 的 RCA 根因分析结果，包括根因、置信度、候选根因、证据和时间线" }
func (t *GetRCATool) ReadOnly() bool             { return true }
func (t *GetRCATool) InputSchema() ToolSchema {
	return ToolSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"incident_id": {Type: "integer", Description: "Incident ID"},
		},
		Required: []string{"incident_id"},
	}
}
func (t *GetRCATool) Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error) {
	id := int64(input["incident_id"].(float64))
	result, err := t.svc.GetLatest(ctx, id)
	if err != nil {
		return ToolResult{ToolName: t.Name(), Success: true, Available: false, Error: "RCA 结果不存在或尚未分析", Source: "mysql", Timestamp: time.Now()}, nil
	}
	return ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Available: true,
		Data:      result,
		Source:    "mysql",
		Timestamp: time.Now(),
	}, nil
}

// ===== get_alerts =====

type GetAlertsTool struct {
	repo *alert.Repository
}

func NewGetAlertsTool(repo *alert.Repository) *GetAlertsTool { return &GetAlertsTool{repo: repo} }
func (t *GetAlertsTool) Name() string                         { return "get_alerts" }
func (t *GetAlertsTool) Description() string                  { return "获取当前活跃的告警列表，可按 namespace、severity、status 筛选" }
func (t *GetAlertsTool) ReadOnly() bool                       { return true }
func (t *GetAlertsTool) InputSchema() ToolSchema {
	return ToolSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"namespace": {Type: "string", Description: "命名空间筛选"},
			"severity":  {Type: "string", Description: "严重程度: critical/warning/info"},
			"status":    {Type: "string", Description: "状态: firing/resolved"},
		},
	}
}
func (t *GetAlertsTool) Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error) {
	filter := alert.ListFilter{}
	if ns, ok := input["namespace"].(string); ok {
		filter.Namespace = ns
	}
	if sev, ok := input["severity"].(string); ok {
		filter.Severity = sev
	}
	if st, ok := input["status"].(string); ok {
		filter.Status = st
	}
	alerts, total, err := t.repo.List(ctx, filter, 1, 50)
	if err != nil {
		return ToolResult{ToolName: t.Name(), Success: false, Error: err.Error(), Source: "mysql", Timestamp: time.Now()}, nil
	}
	return ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Available: true,
		Data:      map[string]interface{}{"alerts": alerts, "total": total},
		Source:    "mysql",
		Timestamp: time.Now(),
	}, nil
}

// ===== get_anomalies =====

type GetAnomaliesTool struct {
	repo *anomaly.Repository
}

func NewGetAnomaliesTool(repo *anomaly.Repository) *GetAnomaliesTool { return &GetAnomaliesTool{repo: repo} }
func (t *GetAnomaliesTool) Name() string                              { return "get_anomalies" }
func (t *GetAnomaliesTool) Description() string                       { return "获取异常检测记录，可按资源、严重程度、时间范围筛选" }
func (t *GetAnomaliesTool) ReadOnly() bool                            { return true }
func (t *GetAnomaliesTool) InputSchema() ToolSchema {
	return ToolSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"cluster":      {Type: "string", Description: "集群名"},
			"resource_type": {Type: "string", Description: "资源类型: pod/node/deployment"},
			"resource_name": {Type: "string", Description: "资源名"},
			"severity":     {Type: "string", Description: "严重程度"},
		},
	}
}
func (t *GetAnomaliesTool) Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error) {
	filter := anomaly.ListFilter{}
	if c, ok := input["cluster"].(string); ok {
		filter.Cluster = c
	}
	if rt, ok := input["resource_type"].(string); ok {
		filter.ResourceType = rt
	}
	if rn, ok := input["resource_name"].(string); ok {
		filter.ResourceName = rn
	}
	if sev, ok := input["severity"].(string); ok {
		filter.Severity = sev
	}
	records, total, err := t.repo.List(ctx, filter, 1, 50)
	if err != nil {
		return ToolResult{ToolName: t.Name(), Success: false, Error: err.Error(), Source: "mysql", Timestamp: time.Now()}, nil
	}
	return ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Available: true,
		Data:      map[string]interface{}{"anomalies": records, "total": total},
		Source:    "mysql",
		Timestamp: time.Now(),
	}, nil
}

// ===== query_metrics =====

type QueryMetricsTool struct {
	querier monitoring.Querier
}

func NewQueryMetricsTool(querier monitoring.Querier) *QueryMetricsTool {
	return &QueryMetricsTool{querier: querier}
}
func (t *QueryMetricsTool) Name() string        { return "query_metrics" }
func (t *QueryMetricsTool) Description() string { return "查询 Prometheus 指标，支持即时查询和范围查询。AI 不能直接访问 Prometheus，必须通过此工具" }
func (t *QueryMetricsTool) ReadOnly() bool      { return true }
func (t *QueryMetricsTool) InputSchema() ToolSchema {
	return ToolSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"query": {Type: "string", Description: "PromQL 查询表达式"},
		},
		Required: []string{"query"},
	}
}
func (t *QueryMetricsTool) Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error) {
	if t.querier == nil {
		return ToolResult{ToolName: t.Name(), Success: true, Available: false, Error: "Prometheus 未配置", Source: "prometheus", Timestamp: time.Now()}, nil
	}
	query, _ := input["query"].(string)
	result, err := t.querier.Query(ctx, query, time.Now())
	if err != nil {
		return ToolResult{ToolName: t.Name(), Success: true, Available: false, Error: err.Error(), Source: "prometheus", Timestamp: time.Now()}, nil
	}
	return ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Available: true,
		Data:      result,
		Source:    "prometheus",
		Timestamp: time.Now(),
	}, nil
}

// ===== search_logs =====

type SearchLogsTool struct {
	esClient *logging.Client
}

func NewSearchLogsTool(esClient *logging.Client) *SearchLogsTool {
	return &SearchLogsTool{esClient: esClient}
}
func (t *SearchLogsTool) Name() string        { return "search_logs" }
func (t *SearchLogsTool) Description() string { return "搜索 Elasticsearch 日志，支持 namespace、pod、keyword、level、时间范围筛选" }
func (t *SearchLogsTool) ReadOnly() bool      { return true }
func (t *SearchLogsTool) InputSchema() ToolSchema {
	return ToolSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"namespace": {Type: "string", Description: "命名空间"},
			"pod":       {Type: "string", Description: "Pod 名"},
			"keyword":   {Type: "string", Description: "搜索关键词"},
			"level":     {Type: "string", Description: "日志级别: error/warn/info/debug"},
		},
	}
}
func (t *SearchLogsTool) Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error) {
	if t.esClient == nil {
		return ToolResult{ToolName: t.Name(), Success: true, Available: false, Error: "Elasticsearch 未配置", Source: "elasticsearch", Timestamp: time.Now()}, nil
	}
	q := logging.SearchQuery{Size: 30}
	if ns, ok := input["namespace"].(string); ok {
		q.Namespace = ns
	}
	if pod, ok := input["pod"].(string); ok {
		q.Pod = pod
	}
	if kw, ok := input["keyword"].(string); ok {
		q.Keyword = kw
	}
	if lv, ok := input["level"].(string); ok {
		q.Level = lv
	}
	q.Start = time.Now().Add(-1 * time.Hour)
	q.End = time.Now()
	result, err := t.esClient.Search(ctx, q)
	if err != nil {
		return ToolResult{ToolName: t.Name(), Success: true, Available: false, Error: err.Error(), Source: "elasticsearch", Timestamp: time.Now()}, nil
	}
	return ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Available: true,
		Data:      result,
		Source:    "elasticsearch",
		Timestamp: time.Now(),
	}, nil
}

// ===== get_k8s_resource =====

type GetK8sResourceTool struct {
	clusterSvc *cluster.Service
}

func NewGetK8sResourceTool(svc *cluster.Service) *GetK8sResourceTool {
	return &GetK8sResourceTool{clusterSvc: svc}
}
func (t *GetK8sResourceTool) Name() string { return "get_k8s_resource" }
func (t *GetK8sResourceTool) Description() string {
	return "获取 Kubernetes 资源详情，支持 pod/deployment/service/node。返回真实 K8s API 数据"
}
func (t *GetK8sResourceTool) ReadOnly() bool { return true }
func (t *GetK8sResourceTool) InputSchema() ToolSchema {
	return ToolSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"cluster":      {Type: "string", Description: "集群名"},
			"namespace":    {Type: "string", Description: "命名空间"},
			"resource_type": {Type: "string", Description: "资源类型: pod/deployment/service/node", Enum: []string{"pod", "deployment", "service", "node"}},
			"resource_name": {Type: "string", Description: "资源名"},
		},
		Required: []string{"cluster", "resource_type", "resource_name"},
	}
}
func (t *GetK8sResourceTool) Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error) {
	if t.clusterSvc == nil {
		return ToolResult{ToolName: t.Name(), Success: true, Available: false, Error: "Kubernetes 未配置", Source: "kubernetes", Timestamp: time.Now()}, nil
	}
	clusterName, _ := input["cluster"].(string)
	namespace, _ := input["namespace"].(string)
	resType, _ := input["resource_type"].(string)
	resName, _ := input["resource_name"].(string)

	var data interface{}
	var err error
	switch resType {
	case "pod":
		data, err = t.clusterSvc.GetPod(ctx, clusterName, namespace, resName)
	case "deployment":
		data, err = t.clusterSvc.GetDeployment(ctx, clusterName, namespace, resName)
	case "node":
		data, err = t.clusterSvc.GetNode(ctx, clusterName, resName)
	default:
		return ToolResult{ToolName: t.Name(), Success: false, Error: fmt.Sprintf("不支持的资源类型: %s", resType), Source: "kubernetes", Timestamp: time.Now()}, nil
	}
	if err != nil {
		return ToolResult{ToolName: t.Name(), Success: false, Error: err.Error(), Source: "kubernetes", Timestamp: time.Now()}, nil
	}
	return ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Available: true,
		Data:      data,
		Source:    "kubernetes",
		Timestamp: time.Now(),
	}, nil
}

// ===== get_k8s_events =====

type GetK8sEventsTool struct {
	clusterSvc *cluster.Service
}

func NewGetK8sEventsTool(svc *cluster.Service) *GetK8sEventsTool {
	return &GetK8sEventsTool{clusterSvc: svc}
}
func (t *GetK8sEventsTool) Name() string        { return "get_k8s_events" }
func (t *GetK8sEventsTool) Description() string { return "获取 Kubernetes Event，支持 Pod 和 Node。OOMKilled/CrashLoopBackOff 等事件是根因分析的强证据" }
func (t *GetK8sEventsTool) ReadOnly() bool      { return true }
func (t *GetK8sEventsTool) InputSchema() ToolSchema {
	return ToolSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"cluster":      {Type: "string", Description: "集群名"},
			"namespace":    {Type: "string", Description: "命名空间"},
			"resource_type": {Type: "string", Description: "资源类型: pod/node", Enum: []string{"pod", "node"}},
			"resource_name": {Type: "string", Description: "资源名"},
		},
		Required: []string{"cluster", "resource_type", "resource_name"},
	}
}
func (t *GetK8sEventsTool) Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error) {
	if t.clusterSvc == nil {
		return ToolResult{ToolName: t.Name(), Success: true, Available: false, Error: "Kubernetes 未配置", Source: "kubernetes", Timestamp: time.Now()}, nil
	}
	clusterName, _ := input["cluster"].(string)
	namespace, _ := input["namespace"].(string)
	resType, _ := input["resource_type"].(string)
	resName, _ := input["resource_name"].(string)

	var events interface{}
	var err error
	if resType == "pod" {
		events, err = t.clusterSvc.GetPodEvents(ctx, clusterName, namespace, resName)
	} else if resType == "node" {
		events, err = t.clusterSvc.GetNodeEvents(ctx, clusterName, resName)
	} else {
		return ToolResult{ToolName: t.Name(), Success: false, Error: "只支持 pod 和 node 事件", Source: "kubernetes", Timestamp: time.Now()}, nil
	}
	if err != nil {
		return ToolResult{ToolName: t.Name(), Success: false, Error: err.Error(), Source: "kubernetes", Timestamp: time.Now()}, nil
	}
	return ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Available: true,
		Data:      events,
		Source:    "kubernetes",
		Timestamp: time.Now(),
	}, nil
}

// ===== get_topology =====

type GetTopologyTool struct {
	topologySvc *topology.Service
}

func NewGetTopologyTool(svc *topology.Service) *GetTopologyTool {
	return &GetTopologyTool{topologySvc: svc}
}
func (t *GetTopologyTool) Name() string        { return "get_topology" }
func (t *GetTopologyTool) Description() string { return "获取 Kubernetes 资源拓扑图，包括节点、边和依赖/影响分析" }
func (t *GetTopologyTool) ReadOnly() bool      { return true }
func (t *GetTopologyTool) InputSchema() ToolSchema {
	return ToolSchema{
		Type: "object",
		Properties: map[string]ToolProperty{
			"cluster":   {Type: "string", Description: "集群名"},
			"namespace": {Type: "string", Description: "命名空间（可选）"},
		},
		Required: []string{"cluster"},
	}
}
func (t *GetTopologyTool) Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error) {
	if t.topologySvc == nil {
		return ToolResult{ToolName: t.Name(), Success: true, Available: false, Error: "Topology 服务未配置", Source: "topology", Timestamp: time.Now()}, nil
	}
	clusterName, _ := input["cluster"].(string)
	namespace, _ := input["namespace"].(string)
	graph, err := t.topologySvc.GetGraph(ctx, clusterName, namespace, false)
	if err != nil {
		return ToolResult{ToolName: t.Name(), Success: false, Error: err.Error(), Source: "topology", Timestamp: time.Now()}, nil
	}
	return ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Available: true,
		Data:      graph,
		Source:    "topology",
		Timestamp: time.Now(),
	}, nil
}

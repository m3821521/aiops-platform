package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aiops/aiops-platform/internal/ai"
	"github.com/aiops/aiops-platform/internal/ai/tools"
	"github.com/aiops/aiops-platform/internal/alert"
	"github.com/aiops/aiops-platform/internal/anomaly"
	"github.com/aiops/aiops-platform/internal/api"
	"github.com/aiops/aiops-platform/internal/audit"
	"github.com/aiops/aiops-platform/internal/auth"
	"github.com/aiops/aiops-platform/internal/automation"
	"github.com/aiops/aiops-platform/internal/cluster"
	"github.com/aiops/aiops-platform/internal/config"
	"github.com/aiops/aiops-platform/internal/handler"
	"github.com/aiops/aiops-platform/internal/infra"
	"github.com/aiops/aiops-platform/internal/incident"
	"github.com/aiops/aiops-platform/internal/logging"
	"github.com/aiops/aiops-platform/internal/migration"
	"github.com/aiops/aiops-platform/internal/monitoring"
	"github.com/aiops/aiops-platform/internal/rca"
	"github.com/aiops/aiops-platform/internal/redisutil"
	"github.com/aiops/aiops-platform/internal/topology"
	"github.com/aiops/aiops-platform/internal/workflow"
	"github.com/aiops/aiops-platform/pkg/logger"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
)

func main() {
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	logger.New(cfg.Log.Level)

	db, err := infra.NewMySQL(cfg.Mysql)
	if err != nil {
		slog.Error("mysql", "err", err)
		os.Exit(1)
	}
	rdb, err := infra.NewRedis(cfg.Redis)
	if err != nil {
		slog.Error("redis", "err", err)
		os.Exit(1)
	}

	clusters, err := cluster.LoadRegistry(cfg.Cluster.ConfigPath)
	if err != nil {
		slog.Warn("load clusters", "err", err)
		clusters = []cluster.Cluster{}
	}
	mgr := cluster.NewManager(clusters)

	// Prometheus 是可选依赖：创建失败只警告，不影响进程启动。
	// 用 Redis 缓存包装查询结果，降低 Prometheus 压力。
	var metricsHandler *handler.MetricsHandler
	var anomalyHandler *handler.AnomalyHandler
	var anomalyService *anomaly.Service
	var anomalyScheduler *anomaly.Scheduler
	var querier monitoring.Querier
	promClient, err := monitoring.NewClient(cfg.Prometheus.Address, time.Duration(cfg.Prometheus.Timeout)*time.Second)
	if err != nil {
		slog.Warn("prometheus client disabled", "err", err)
	} else {
		querier = monitoring.NewCachedQuerier(promClient, rdb)
		metricsHandler = &handler.MetricsHandler{Prom: querier}
		anomalyRepo := anomaly.NewRepository(db)
		anomalyService = anomaly.NewServiceWithRepo(querier, anomalyRepo)
		anomalyHandler = &handler.AnomalyHandler{Service: anomalyService, Repo: anomalyRepo}
		slog.Info("prometheus connected", "addr", cfg.Prometheus.Address)
	}

	alertRepo := alert.NewRepository(db)
	alertAggregator := alert.NewAggregator(alertRepo)
	alertNoiseReducer := alert.NewNoiseReducer(alertRepo)

	// Incident 事件中心。
	incidentRepo := incident.NewRepository(db)
	incidentCorrelator := incident.NewCorrelator(incident.DefaultCorrelationConfig())
	incidentService := incident.NewService(incidentRepo, incidentCorrelator)
	incidentHandler := &incident.Handler{Service: incidentService}

	// 数据库迁移（开发环境使用 AutoMigrate，未来替换为 versioned migration）。
	migrator := migration.NewGormMigrator(db)
	if err := migrator.Migrate(&incident.Incident{}, &incident.IncidentSignal{}, &anomaly.AnomalyRecord{}, &rca.IncidentAnalysis{}, &ai.AIAnalysisRecord{}, &tools.ToolAuditRecord{}, &automation.Action{}, &automation.ActionExecution{}, &automation.AutomationAudit{}, &workflow.Workflow{}, &workflow.WorkflowStep{}); err != nil {
		slog.Warn("migration failed", "err", err)
	}

	// Anomaly → Incident 集成：适配器将 anomaly.AnomalySignal 转换为 incident.Signal。
	if anomalyService != nil {
		anomalyService.SetIncidentSink(&anomalyIncidentAdapter{incidentSvc: incidentService})
		// 启动定时检测调度器（可配置，默认规则）。
		anomalyScheduler = anomaly.NewScheduler(anomalyService, anomaly.DefaultRules())
		anomalyScheduler.Start(context.Background())
		slog.Info("anomaly scheduler started", "rules", len(anomaly.DefaultRules()))
	}

	// Topology 拓扑服务。
	topologyProvider := topology.NewClusterManagerProvider(mgr)
	topologyBuilder := topology.NewBuilder(topologyProvider)
	var anomalyRepoForTopology *anomaly.Repository
	if anomalyHandler != nil {
		anomalyRepoForTopology = anomalyHandler.Repo
	}
	topologyStatusProvider := topology.NewDefaultStatusProvider(incidentRepo, anomalyRepoForTopology)
	topologyService := topology.NewService(topologyBuilder, rdb, topologyStatusProvider)
	topologyHandler := &topology.Handler{Service: topologyService}
	slog.Info("topology service initialized")

	rcaEngine := rca.NewEngine()
	esClient := logging.NewClient(cfg.Elasticsearch.Address, cfg.Elasticsearch.Index,
		cfg.Elasticsearch.Username, cfg.Elasticsearch.Password, cfg.Elasticsearch.Timeout)
	logAnalyzer := logging.NewAnalyzer(10, 1*time.Hour)

	// AI 助手（可选）。
	var aiAssistant *ai.Assistant
	var aiProvider ai.Provider
	if cfg.AI.Enabled {
		aiProvider = ai.NewOpenAIProvider(cfg.AI.BaseURL, cfg.AI.APIKey, cfg.AI.Model, cfg.AI.Timeout)
		aiAssistant = ai.NewAssistant(aiProvider, alertRepo)
		slog.Info("AI assistant enabled", "provider", cfg.AI.Provider, "model", cfg.AI.Model)
	} else {
		slog.Info("AI assistant disabled")
	}

	clusterSvc := cluster.NewService(mgr)
	automationEngine := automation.NewEngine(clusterSvc)
	jenkinsClient := automation.NewJenkinsClient(cfg.Jenkins.URL, cfg.Jenkins.Username, cfg.Jenkins.Token, cfg.Jenkins.Timeout)
	argocdClient := automation.NewArgoCDClient(cfg.ArgoCD.URL, cfg.ArgoCD.Token, cfg.ArgoCD.Timeout)

	// Automation Action Framework（审批+执行+审计）。
	actionRepo := automation.NewActionRepository(db)
	executionRepo := automation.NewExecutionRepository(db)
	automationAuditRepo := automation.NewAuditRepository(db)
	automationPolicy := automation.NewPolicyEngine(cfg.Server.Mode)
	k8sExecutor := automation.NewKubernetesExecutor(clusterSvc)
	jenkinsExecutor := automation.NewJenkinsExecutor(jenkinsClient)
	argocdExecutor := automation.NewArgoCDExecutor(argocdClient)
	automationService := automation.NewService(actionRepo, executionRepo, automationAuditRepo, automationPolicy, k8sExecutor, jenkinsExecutor, argocdExecutor)
	// Incident Timeline 集成：Action 执行完成后写入 Incident 信号。
	automationService.OnExecutionComplete = func(ctx context.Context, incidentID int64, act *automation.Action, result *automation.ExecutionResult) {
		signal := &incident.IncidentSignal{
			IncidentID:   incidentID,
			SignalType:   "automation",
			SignalID:     fmt.Sprintf("action-%d", act.ID),
			Title:        fmt.Sprintf("%s %s", act.ActionType, act.TargetName),
			Severity:     "info",
			Cluster:      act.Cluster,
			Namespace:    act.Namespace,
			ResourceType: act.TargetType,
			ResourceName: act.TargetName,
			Timestamp:    time.Now(),
			Resolved:     result.Success,
			Metadata: incident.JSONMap{
				"action_id": act.ID,
				"action_type": act.ActionType,
				"success": result.Success,
				"message": result.Message,
				"error": result.Error,
			},
		}
		if result.Success {
			now := time.Now()
			signal.ResolvedAt = &now
		}
		_, _ = incidentRepo.UpsertSignal(ctx, signal)
	}
	automationActionHandler := &automation.Handler{Service: automationService}
	slog.Info("automation action framework initialized")

	// Workflow 编排引擎。
	workflowRepo := workflow.NewRepository(db)
	workflowService := workflow.NewService(workflowRepo, &workflowActionAdapter{automationSvc: automationService})
	workflowHandler := &workflow.Handler{Service: workflowService}
	slog.Info("workflow engine initialized")

	// RCA V2 Pipeline（基于 Incident 的完整根因分析）。
	rcaAnalysisRepo := rca.NewAnalysisRepository(db)
	rcaCollector := &rcaContextCollector{
		incidentRepo: incidentRepo,
		anomalyRepo:  anomalyHandler.Repo,
		topologySvc:  topologyService,
		clusterSvc:   clusterSvc,
		querier:      querier,
		esClient:     esClient,
	}
	rcaPipeline := rca.NewPipeline(rcaCollector)
	rcaService := rca.NewService(rcaPipeline, rcaAnalysisRepo)
	incidentRCAHandler := &handler.IncidentRCAHandler{
		RCAService:      rcaService,
		IncidentService: incidentService,
	}
	slog.Info("rca pipeline initialized")

	// AI Analysis（基于 Incident + RCA 的智能分析）。
	var aiAnalysisService *ai.AnalysisService
	var aiAnalysisRepo *ai.AIAnalysisRepository
	if cfg.AI.Enabled && aiProvider != nil {
		aiAnalysisRepo = ai.NewAIAnalysisRepository(db)
		aiContextProv := &aiContextProvider{
			incidentRepo: incidentRepo,
			rcaService:   rcaService,
			anomalyRepo:  anomalyHandler.Repo,
			topologySvc:  topologyService,
			clusterSvc:   clusterSvc,
			querier:      querier,
			esClient:     esClient,
		}
		aiAnalysisService = ai.NewAnalysisService(aiProvider, aiContextProv, cfg.AI.Timeout)
		slog.Info("ai analysis service initialized")
	}
	incidentAIHandler := &ai.IncidentAIHandler{
		Service:    aiAnalysisService,
		Repository: aiAnalysisRepo,
		Enabled:    cfg.AI.Enabled && aiProvider != nil,
	}

	// AI Tool Calling Engine（只读工具，依赖 RCA/Topology/Cluster 等服务）。
	var toolEngine *tools.Engine
	var toolAuditRepo *tools.ToolAuditRepository
	if cfg.AI.Enabled && aiProvider != nil {
		toolRegistry := tools.NewRegistry()
		_ = toolRegistry.Register(tools.NewGetIncidentTool(incidentRepo))
		_ = toolRegistry.Register(tools.NewGetRCATool(rcaService))
		_ = toolRegistry.Register(tools.NewGetAlertsTool(alertRepo))
		_ = toolRegistry.Register(tools.NewGetAnomaliesTool(anomalyHandler.Repo))
		_ = toolRegistry.Register(tools.NewQueryMetricsTool(querier))
		_ = toolRegistry.Register(tools.NewSearchLogsTool(esClient))
		_ = toolRegistry.Register(tools.NewGetK8sResourceTool(clusterSvc))
		_ = toolRegistry.Register(tools.NewGetK8sEventsTool(clusterSvc))
		_ = toolRegistry.Register(tools.NewGetTopologyTool(topologyService))
		toolEngine = tools.NewEngine(aiProvider, toolRegistry, tools.DefaultEngineConfig())
		toolAuditRepo = tools.NewToolAuditRepository(db)
		slog.Info("ai tool calling engine initialized", "tools", len(toolRegistry.List()))
	}

	// 认证服务。
	userRepo := auth.NewRepository(db)
	authService := auth.NewService(userRepo, cfg.Auth.JWTSecret, time.Duration(cfg.Auth.JWTExpiration)*time.Hour)
	auditRepo := audit.NewRepository(db)
	rateLimiter := redisutil.NewRateLimiter(rdb, 100, time.Minute)

	router := api.NewRouter(cfg.Server.Mode, api.Deps{
		Health:     &handler.HealthHandler{DB: db, Redis: rdb},
		Cluster:    &handler.ClusterHandler{Service: clusterSvc},
		Metrics:    metricsHandler,
		Alert:      &handler.AlertHandler{Repo: alertRepo, Aggregator: alertAggregator, NoiseReducer: alertNoiseReducer, IncidentService: incidentService},
		Anomaly:    anomalyHandler,
		RCA:        &handler.RCAHandler{AlertRepo: alertRepo, Engine: rcaEngine},
		Logs:       &handler.LogsHandler{ES: esClient, Analyzer: logAnalyzer},
		AI: &handler.AIHandler{Assistant: aiAssistant, Engine: toolEngine, AuditRepo: toolAuditRepo, Enabled: cfg.AI.Enabled},
		Automation: &handler.AutomationHandler{Engine: automationEngine},
		AutomationAction: automationActionHandler,
		Workflow:        workflowHandler,
		Jenkins:    &handler.JenkinsHandler{Jenkins: jenkinsClient},
		ArgoCD:     &handler.ArgoCDHandler{ArgoCD: argocdClient},
		Auth:       &handler.AuthHandler{AuthService: authService, UserRepo: userRepo},
		AuthService: authService,
		Audit:      &handler.AuditHandler{Repo: auditRepo},
		Incident:   incidentHandler,
		IncidentRCA: incidentRCAHandler,
		IncidentAI:  incidentAIHandler,
		Topology:   topologyHandler,
		RateLimit:  rateLimiter,
	})

	addr := cfg.Server.Addr()
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
		// 防止慢连接占满 goroutine，ReadHeaderTimeout 是 http.Server 推荐设置。
		ReadHeaderTimeout: 10 * time.Second,
	}

	// 后台启动 HTTP 服务。
	go func() {
		slog.Info("aiops-platform started", "addr", addr, "clusters", len(clusters))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server", "err", err)
			os.Exit(1)
		}
	}()

	// 等待中断信号（Ctrl+C 或 Kubernetes 发送的 SIGTERM）。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down...")

	// 最多等 30 秒让在途请求处理完，超时强制退出。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server shutdown", "err", err)
	}

	// 关闭基础设施连接。
	if anomalyScheduler != nil {
		anomalyScheduler.Stop()
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	_ = rdb.Close()

	slog.Info("aiops-platform stopped")
}

// anomalyIncidentAdapter 是 anomaly.IncidentSink 接口的适配器，
// 将 anomaly.AnomalySignal 转换为 incident.Signal 并接入 Incident 引擎。
// 通过适配器避免 anomaly → incident 循环依赖。
type anomalyIncidentAdapter struct {
	incidentSvc *incident.Service
}

func (a *anomalyIncidentAdapter) IngestAnomalySignal(ctx context.Context, sig anomaly.AnomalySignal) (int64, error) {
	if a.incidentSvc == nil {
		return 0, nil
	}
	incidentSig := incident.Signal{
		SignalType:   incident.SignalAnomaly,
		SignalID:     fmt.Sprintf("anomaly-%d", sig.ID),
		Title:        sig.Title,
		Severity:     sig.Severity,
		Cluster:      sig.Cluster,
		Namespace:    sig.Namespace,
		Service:      sig.Service,
		ResourceType: incident.ResourceType(sig.ResourceType),
		ResourceName: sig.ResourceName,
		Timestamp:    sig.Timestamp,
		Resolved:     sig.Resolved,
		Metadata: map[string]any{
			"metric":        sig.Metric,
			"value":         sig.Value,
			"anomaly_score": sig.AnomalyScore,
			"reason":        sig.Reason,
			"algorithm":     sig.Algorithm,
		},
	}
	inc, _, err := a.incidentSvc.IngestSignal(ctx, incidentSig)
	if err != nil {
		return 0, err
	}
	if inc != nil {
		return inc.ID, nil
	}
	return 0, nil
}

// workflowActionAdapter 让 automation.Service 实现 workflow.ActionExecutor 接口。
type workflowActionAdapter struct {
	automationSvc *automation.Service
}

func (a *workflowActionAdapter) ExecuteAction(ctx context.Context, actionType, cluster, namespace, targetName string, params map[string]interface{}) (bool, string, error) {
	if a.automationSvc == nil {
		return false, "automation service unavailable", nil
	}
	// 创建临时 Action 并执行（不经过审批，因为 Workflow 已经审批）
	act := &automation.Action{
		ActionType: actionType,
		Cluster:    cluster,
		Namespace:  namespace,
		TargetName: targetName,
		Status:     automation.StatusApproved,
	}
	act.SetParameters(params)
	result, err := a.automationSvc.ExecuteWorkflowStep(ctx, act)
	if err != nil {
		return false, err.Error(), nil
	}
	return result.Success, result.Message, nil
}

// rcaContextCollector 是 rca.ContextCollector 接口的适配器，
// 从 Incident/Anomaly/Topology/Cluster 等服务收集 RCA 上下文。
// 通过适配器避免 rca → 各业务包的循环依赖。
type rcaContextCollector struct {
	incidentRepo *incident.Repository
	anomalyRepo  *anomaly.Repository
	topologySvc  *topology.Service
	clusterSvc   *cluster.Service
	querier      monitoring.Querier
	esClient     *logging.Client
}

func (c *rcaContextCollector) CollectAlerts(ctx context.Context, incidentID int64) ([]rca.AlertInfo, error) {
	signals, err := c.incidentRepo.ListSignals(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	var alerts []rca.AlertInfo
	for _, s := range signals {
		if s.SignalType != "alert" {
			continue
		}
		a := rca.AlertInfo{
			ID:          s.ID,
			Fingerprint: s.SignalID,
			Alertname:   s.Title,
			Severity:    s.Severity,
			Service:     s.Service,
			Namespace:   s.Namespace,
			StartsAt:    s.Timestamp,
		}
		if s.ResourceType == "pod" {
			a.Pod = s.ResourceName
		} else if s.ResourceType == "node" {
			a.Node = s.ResourceName
		}
		alerts = append(alerts, a)
	}
	return alerts, nil
}

func (c *rcaContextCollector) CollectAnomalies(ctx context.Context, cluster, namespace, resourceType, resourceName string, since time.Time) ([]rca.AnomalyInfo, error) {
	if c.anomalyRepo == nil || resourceName == "" {
		return nil, nil
	}
	records, err := c.anomalyRepo.FindByResource(ctx, cluster, resourceType, resourceName, since)
	if err != nil {
		return nil, err
	}
	var anomalies []rca.AnomalyInfo
	for _, r := range records {
		anomalies = append(anomalies, rca.AnomalyInfo{
			ID:           r.ID,
			Metric:       r.Metric,
			ResourceType: r.ResourceType,
			ResourceName: r.ResourceName,
			Namespace:    r.Namespace,
			Timestamp:    r.Timestamp,
			Value:        r.Value,
			Baseline:     r.Baseline,
			AnomalyScore: r.AnomalyScore,
			Severity:     r.Severity,
			Algorithm:    r.Algorithm,
			Reason:       r.Reason,
		})
	}
	return anomalies, nil
}

func (c *rcaContextCollector) CollectEvents(ctx context.Context, cluster, namespace, resourceType, resourceName string, since time.Time) ([]rca.EventInfo, error) {
	if c.clusterSvc == nil || resourceName == "" {
		return nil, nil
	}
	var events []rca.EventInfo
	if resourceType == "pod" {
		k8sEvents, err := c.clusterSvc.GetPodEvents(ctx, cluster, namespace, resourceName)
		if err != nil {
			return nil, err
		}
		for _, e := range k8sEvents {
			if e.LastTimestamp.Time.Before(since) {
				continue
			}
			events = append(events, rca.EventInfo{
				Type:         e.Type,
				Reason:       e.Reason,
				Message:      e.Message,
				ResourceType: "pod",
				ResourceName: resourceName,
				Namespace:    namespace,
				Timestamp:    e.LastTimestamp.Time,
				Count:        e.Count,
			})
		}
	} else if resourceType == "node" {
		k8sEvents, err := c.clusterSvc.GetNodeEvents(ctx, cluster, resourceName)
		if err != nil {
			return nil, err
		}
		for _, e := range k8sEvents {
			if e.LastTimestamp.Time.Before(since) {
				continue
			}
			events = append(events, rca.EventInfo{
				Type:         e.Type,
				Reason:       e.Reason,
				Message:      e.Message,
				ResourceType: "node",
				ResourceName: resourceName,
				Timestamp:    e.LastTimestamp.Time,
				Count:        e.Count,
			})
		}
	}
	return events, nil
}

func (c *rcaContextCollector) CollectMetrics(ctx context.Context, cluster, namespace, resourceType, resourceName string, since, until time.Time) ([]rca.MetricInfo, error) {
	if c.querier == nil || resourceName == "" {
		return nil, nil
	}
	var metrics []rca.MetricInfo
	// 根据资源类型选择指标查询。
	queries := map[string]string{}
	if resourceType == "pod" {
		queries["container_cpu"] = fmt.Sprintf(`rate(container_cpu_usage_seconds_total{pod="%s",namespace="%s"}[5m])`, resourceName, namespace)
		queries["container_memory"] = fmt.Sprintf(`container_memory_usage_bytes{pod="%s",namespace="%s"}`, resourceName, namespace)
		queries["container_restart"] = fmt.Sprintf(`kube_pod_container_status_restarts_total{pod="%s",namespace="%s"}`, resourceName, namespace)
	} else if resourceType == "node" {
		queries["node_cpu"] = fmt.Sprintf(`100 - (avg by(instance) (rate(node_cpu_seconds_total{mode="idle",instance=~"%s.*"}[5m])) * 100)`, resourceName)
		queries["node_memory"] = fmt.Sprintf(`(1 - node_memory_MemAvailable_bytes{instance=~"%s.*"} / node_memory_MemTotal_bytes{instance=~"%s.*"}) * 100`, resourceName, resourceName)
	}
	for name, promQL := range queries {
		result, err := c.querier.Query(ctx, promQL, until)
		if err != nil {
			continue // 单个指标失败不阻塞。
		}
		if result == nil {
			continue
		}
		// 解析 vector 结果。
		if vec, ok := result.Result.([]model.Sample); ok {
			for _, s := range vec {
				metrics = append(metrics, rca.MetricInfo{
					Metric:    name,
					Value:     float64(s.Value),
					Timestamp: s.Timestamp.Time(),
					Resource:  resourceName,
				})
			}
		}
	}
	return metrics, nil
}

func (c *rcaContextCollector) CollectLogs(ctx context.Context, cluster, namespace, pod string, since, until time.Time) ([]rca.LogInfo, error) {
	if c.esClient == nil || pod == "" {
		return nil, nil
	}
	result, err := c.esClient.Search(ctx, logging.SearchQuery{
		Namespace: namespace,
		Pod:       pod,
		Level:     "error",
		Start:     since,
		End:       until,
		Size:      50,
	})
	if err != nil {
		return nil, nil // ES 不可用时优雅降级。
	}
	var logs []rca.LogInfo
	for _, hit := range result.Hits {
		logs = append(logs, rca.LogInfo{
			Timestamp: hit.Timestamp,
			Level:     hit.Level,
			Message:   hit.Message,
			Pod:       hit.Pod,
			Namespace: hit.Namespace,
		})
	}
	return logs, nil
}

func (c *rcaContextCollector) CollectTopology(ctx context.Context, cluster, namespace string) (rca.TopologyInfo, error) {
	if c.topologySvc == nil {
		return rca.TopologyInfo{}, nil
	}
	graph, err := c.topologySvc.GetGraph(ctx, cluster, namespace, false)
	if err != nil {
		return rca.TopologyInfo{}, err
	}
	info := rca.TopologyInfo{}
	for _, n := range graph.Nodes {
		info.Nodes = append(info.Nodes, rca.TopologyNodeInfo{
			ID:        n.ID,
			Type:      string(n.Type),
			Name:      n.Name,
			Namespace: n.Namespace,
			Status:    string(n.Status),
		})
	}
	for _, e := range graph.Edges {
		info.Edges = append(info.Edges, rca.TopologyEdgeInfo{
			Source:   e.Source,
			Target:   e.Target,
			Relation: string(e.Relation),
		})
	}
	return info, nil
}

// aiContextProvider 是 ai.AIContextProvider 接口的适配器。
// 从 Incident/RCA/Anomaly/Topology/Metrics/Logs/Events 收集 AI 上下文。
type aiContextProvider struct {
	incidentRepo *incident.Repository
	rcaService   *rca.Service
	anomalyRepo  *anomaly.Repository
	topologySvc  *topology.Service
	clusterSvc   *cluster.Service
	querier      monitoring.Querier
	esClient     *logging.Client
}

func (p *aiContextProvider) BuildContext(ctx context.Context, incidentID int64) (*ai.AIContext, error) {
	// 1. 获取 Incident。
	inc, err := p.incidentRepo.FindByID(ctx, incidentID)
	if err != nil {
		return nil, fmt.Errorf("获取 incident 失败: %w", err)
	}

	endTime := time.Now()
	if inc.EndTime != nil {
		endTime = *inc.EndTime
	}
	since := inc.StartTime.Add(-30 * time.Minute)

	aiCtx := &ai.AIContext{
		IncidentID:       inc.ID,
		Cluster:          inc.Cluster,
		Namespace:        inc.Namespace,
		Service:          inc.Service,
		ResourceType:     inc.ResourceType,
		ResourceName:     inc.ResourceName,
		StartTime:        inc.StartTime,
		EndTime:          endTime,
		IncidentTitle:    inc.Title,
		IncidentSeverity: inc.Severity,
	}

	// 2. 获取 RCA 结果。
	if p.rcaService != nil {
		if rcaResult, err := p.rcaService.GetLatest(ctx, incidentID); err == nil && rcaResult != nil {
			aiCtx.RCA = &ai.RCASummary{
				RootCause:     rcaResult.RootCause,
				Confidence:    rcaResult.Confidence,
				Status:        string(rcaResult.Status),
				EvidenceCount: len(rcaResult.Evidence),
			}
			aiCtx.DataSources.RcaAvailable = true
		}
	}

	// 3. 获取 Alerts（从 incident signals）。
	if signals, err := p.incidentRepo.ListSignals(ctx, incidentID); err == nil {
		for _, s := range signals {
			if s.SignalType == "alert" {
				aiCtx.Alerts = append(aiCtx.Alerts, ai.AlertSummary{
					Name:      s.Title,
					Severity:  s.Severity,
					Service:   s.Service,
					Pod:       s.ResourceName,
					Namespace: s.Namespace,
					StartsAt:  s.Timestamp,
				})
			}
		}
		aiCtx.DataSources.AlertsAvailable = len(aiCtx.Alerts) > 0
	}

	// 4. 获取 Anomalies。
	if p.anomalyRepo != nil && inc.ResourceName != "" {
		if anomalies, err := p.anomalyRepo.FindByResource(ctx, inc.Cluster, inc.ResourceType, inc.ResourceName, since); err == nil {
			for _, a := range anomalies {
				aiCtx.Anomalies = append(aiCtx.Anomalies, ai.AnomalySummary{
					Metric:       a.Metric,
					ResourceType: a.ResourceType,
					ResourceName: a.ResourceName,
					Value:        a.Value,
					Baseline:     a.Baseline,
					Score:        a.AnomalyScore,
					Severity:     a.Severity,
					Algorithm:    a.Algorithm,
					Reason:       a.Reason,
					Timestamp:    a.Timestamp,
				})
			}
			aiCtx.DataSources.AnomaliesAvailable = len(aiCtx.Anomalies) > 0
		}
	}

	// 5. 获取 Metrics。
	if p.querier != nil && inc.ResourceName != "" {
		queries := buildMetricQueries(inc.ResourceType, inc.ResourceName, inc.Namespace)
		for name, promQL := range queries {
			if result, err := p.querier.Query(ctx, promQL, endTime); err == nil && result != nil {
				if vec, ok := result.Result.([]model.Sample); ok {
					for _, s := range vec {
						aiCtx.Metrics = append(aiCtx.Metrics, ai.MetricSummary{
							Metric:    name,
							Value:     float64(s.Value),
							Resource:  inc.ResourceName,
							Timestamp: s.Timestamp.Time(),
						})
					}
				}
			}
		}
		aiCtx.DataSources.MetricsAvailable = len(aiCtx.Metrics) > 0
	}

	// 6. 获取 Logs。
	if p.esClient != nil && inc.ResourceType == "pod" {
		if result, err := p.esClient.Search(ctx, logging.SearchQuery{
			Namespace: inc.Namespace,
			Pod:       inc.ResourceName,
			Level:     "error",
			Start:     since,
			End:       endTime,
			Size:      30,
		}); err == nil {
			for _, hit := range result.Hits {
				aiCtx.Logs = append(aiCtx.Logs, ai.LogSummary{
					Timestamp: hit.Timestamp,
					Level:     hit.Level,
					Message:   hit.Message,
					Pod:       hit.Pod,
					Namespace: hit.Namespace,
				})
			}
			aiCtx.DataSources.LogsAvailable = len(aiCtx.Logs) > 0
		}
	}

	// 7. 获取 Kubernetes Events。
	if p.clusterSvc != nil && inc.ResourceName != "" {
		var events []corev1.Event
		if inc.ResourceType == "pod" {
			events, _ = p.clusterSvc.GetPodEvents(ctx, inc.Cluster, inc.Namespace, inc.ResourceName)
		} else if inc.ResourceType == "node" {
			events, _ = p.clusterSvc.GetNodeEvents(ctx, inc.Cluster, inc.ResourceName)
		}
		for _, e := range events {
			if e.LastTimestamp.Time.Before(since) {
				continue
			}
			aiCtx.Events = append(aiCtx.Events, ai.EventSummary{
				Type:         e.Type,
				Reason:       e.Reason,
				Message:      e.Message,
				ResourceType: inc.ResourceType,
				ResourceName: inc.ResourceName,
				Namespace:    inc.Namespace,
				Timestamp:    e.LastTimestamp.Time,
				Count:        e.Count,
			})
		}
		aiCtx.DataSources.EventsAvailable = len(aiCtx.Events) > 0
	}

	// 8. 获取 Topology（只保留相关子图）。
	if p.topologySvc != nil {
		if graph, err := p.topologySvc.GetGraph(ctx, inc.Cluster, inc.Namespace, false); err == nil {
			aiCtx.Topology = &ai.TopologySummary{
				NodeCount: len(graph.Nodes),
				EdgeCount: len(graph.Edges),
			}
			// 节点少于 30 时发送全量。
			if len(graph.Nodes) <= 30 {
				for _, n := range graph.Nodes {
					aiCtx.Topology.Nodes = append(aiCtx.Topology.Nodes, ai.TopologyNodeInfo{
						ID:        n.ID,
						Type:      string(n.Type),
						Name:      n.Name,
						Namespace: n.Namespace,
						Status:    string(n.Status),
					})
				}
				for _, e := range graph.Edges {
					aiCtx.Topology.Edges = append(aiCtx.Topology.Edges, ai.TopologyEdgeInfo{
						Source:   e.Source,
						Target:   e.Target,
						Relation: string(e.Relation),
					})
				}
			}
			aiCtx.DataSources.TopologyAvailable = true
		}
	}

	return aiCtx, nil
}

// buildMetricQueries 根据资源类型构建 PromQL 查询。
func buildMetricQueries(resourceType, resourceName, namespace string) map[string]string {
	queries := map[string]string{}
	if resourceType == "pod" {
		queries["container_cpu"] = fmt.Sprintf(`rate(container_cpu_usage_seconds_total{pod="%s",namespace="%s"}[5m])`, resourceName, namespace)
		queries["container_memory"] = fmt.Sprintf(`container_memory_usage_bytes{pod="%s",namespace="%s"}`, resourceName, namespace)
	} else if resourceType == "node" {
		queries["node_cpu"] = fmt.Sprintf(`100 - (avg by(instance) (rate(node_cpu_seconds_total{mode="idle",instance=~"%s.*"}[5m])) * 100)`, resourceName)
		queries["node_memory"] = fmt.Sprintf(`(1 - node_memory_MemAvailable_bytes{instance=~"%s.*"} / node_memory_MemTotal_bytes{instance=~"%s.*"}) * 100`, resourceName, resourceName)
	}
	return queries
}

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
	"github.com/aiops/aiops-platform/internal/agent"
	"github.com/aiops/aiops-platform/internal/alert"
	"github.com/aiops/aiops-platform/internal/anomaly"
	"github.com/aiops/aiops-platform/internal/api"
	"github.com/aiops/aiops-platform/internal/audit"
	"github.com/aiops/aiops-platform/internal/auth"
	"github.com/aiops/aiops-platform/internal/automation"
	"github.com/aiops/aiops-platform/internal/cluster"
	"github.com/aiops/aiops-platform/internal/config"
	"github.com/aiops/aiops-platform/internal/connection"
	"github.com/aiops/aiops-platform/internal/handler"
	"github.com/aiops/aiops-platform/internal/infra"
	"github.com/aiops/aiops-platform/internal/incident"
	"github.com/aiops/aiops-platform/internal/logging"
	"github.com/aiops/aiops-platform/internal/migration"
	"github.com/aiops/aiops-platform/internal/monitoring"
	"github.com/aiops/aiops-platform/internal/providers"
	"github.com/aiops/aiops-platform/internal/rca"
	"github.com/aiops/aiops-platform/internal/redisutil"
	"github.com/aiops/aiops-platform/internal/secret"
	"github.com/aiops/aiops-platform/internal/topology"
	"github.com/aiops/aiops-platform/internal/workflow"
	"github.com/aiops/aiops-platform/pkg/logger"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
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
	if err := migrator.Migrate(&incident.Incident{}, &incident.IncidentSignal{}, &anomaly.AnomalyRecord{}, &rca.IncidentAnalysis{}, &ai.AIAnalysisRecord{}, &ai.Conversation{}, &ai.ConversationMessage{}, &ai.AIConfig{}, &tools.ToolAuditRecord{}, &automation.Action{}, &automation.ActionExecution{}, &automation.AutomationAudit{}, &workflow.Workflow{}, &workflow.WorkflowStep{}, &workflow.WorkflowExecution{}, &workflow.WorkflowStepExecution{}, &connection.Connection{}, &connection.Credential{}); err != nil {
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
	var openAIProvider *ai.OpenAIProvider
	if cfg.AI.Enabled {
		openAIProvider = ai.NewOpenAIProvider(cfg.AI.BaseURL, cfg.AI.APIKey, cfg.AI.Model, cfg.AI.Timeout)
		aiProvider = openAIProvider
		aiAssistant = ai.NewAssistant(aiProvider, alertRepo)
		slog.Info("AI assistant enabled", "provider", cfg.AI.Provider, "model", cfg.AI.Model)
	} else {
		slog.Info("AI assistant disabled")
	}

	// AI 配置存储（加密存储 API Key，支持前端配置）
	aiConfigSecret := cfg.Auth.JWTSecret
	if len(aiConfigSecret) < 32 {
		aiConfigSecret = "aiops-platform-ai-config-secret-key-32bytes"
	}
	aiConfigRepo := ai.NewAIConfigRepository(db, aiConfigSecret)
	// 从数据库加载已配置的 AI 配置（覆盖配置文件中的默认值）
	if openAIProvider != nil {
		if dbCfg, err := aiConfigRepo.Get(context.Background()); err == nil && dbCfg != nil {
			if dbCfg.BaseURL != "" {
				openAIProvider.SetBaseURL(dbCfg.BaseURL)
			}
			if dbCfg.Model != "" {
				openAIProvider.SetModel(dbCfg.Model)
			}
			if dbAPIKey, err := aiConfigRepo.GetAPIKey(context.Background()); err == nil && dbAPIKey != "" {
				openAIProvider.SetAPIKey(dbAPIKey)
			}
			slog.Info("AI config loaded from database", "provider", dbCfg.Provider, "model", dbCfg.Model, "base_url", dbCfg.BaseURL)
		}
	}
	aiConfigHandler := &handler.AIConfigHandler{
		Repo:     aiConfigRepo,
		Provider: openAIProvider,
	}

	// P5-0 Connection & Credential Manager
	// Secret Provider：使用 AES-256-GCM 加密存储敏感信息
	secretProvider, err := secret.NewLocalEncryptedSecretProvider(aiConfigSecret)
	if err != nil {
		slog.Error("failed to create secret provider", "error", err)
	} else {
		if err := secretProvider.Validate(context.Background()); err != nil {
			slog.Error("secret provider validation failed", "error", err)
		} else {
			slog.Info("secret provider initialized", "type", secretProvider.Type())
		}
	}

	// Connection & Credential Repository
	connectionRepo := connection.NewConnectionRepository(db)
	credentialRepo := connection.NewCredentialRepository(db)

	// Connection & Credential Service
	credentialService := connection.NewCredentialService(credentialRepo, secretProvider)
	connectionService := connection.NewConnectionService(connectionRepo, credentialService)

	// Legacy Config Compatibility Adapter（将旧 config.yaml 映射为 Connection）
	legacyAdapter := connection.NewLegacyConfigAdapter()
	legacyAdapter.Load(cfg)
	slog.Info("legacy config adapter loaded", "connections", legacyAdapter.Count())

	// Connection Manager（整合数据库 Connection 和 Legacy Config）
	connectionManager := connection.NewConnectionManager(connectionService, legacyAdapter)

	// Provider Registry（Phase A 只注册基础结构，具体 Provider 在 Phase B-F 中逐步迁移）
	providerRegistry := connection.NewProviderRegistry()

	// 注册 P5-B Provider Adapters
	k8sProvider := providers.NewKubernetesProvider(credentialService)
	promProvider := providers.NewPrometheusProvider(credentialService)
	esProvider := providers.NewElasticsearchProvider(credentialService)
	jenkinsProvider := providers.NewJenkinsProvider(credentialService)
	argocdProvider := providers.NewArgoCDProvider(credentialService)
	mysqlProvider := providers.NewMySQLProvider(credentialService)
	redisProvider := providers.NewRedisProvider(credentialService)
	grafanaProvider := providers.NewGrafanaProvider(credentialService)
	dockerProvider := providers.NewDockerProvider(credentialService)

	if err := providerRegistry.Register(k8sProvider); err != nil {
		slog.Error("failed to register kubernetes provider", "error", err)
	}
	if err := providerRegistry.Register(promProvider); err != nil {
		slog.Error("failed to register prometheus provider", "error", err)
	}
	if err := providerRegistry.Register(esProvider); err != nil {
		slog.Error("failed to register elasticsearch provider", "error", err)
	}
	if err := providerRegistry.Register(jenkinsProvider); err != nil {
		slog.Error("failed to register jenkins provider", "error", err)
	}
	if err := providerRegistry.Register(argocdProvider); err != nil {
		slog.Error("failed to register argocd provider", "error", err)
	}
	if err := providerRegistry.Register(mysqlProvider); err != nil {
		slog.Error("failed to register mysql provider", "error", err)
	}
	if err := providerRegistry.Register(redisProvider); err != nil {
		slog.Error("failed to register redis provider", "error", err)
	}
	if err := providerRegistry.Register(grafanaProvider); err != nil {
		slog.Error("failed to register grafana provider", "error", err)
	}
	if err := providerRegistry.Register(dockerProvider); err != nil {
		slog.Error("failed to register docker provider", "error", err)
	}

	slog.Info("provider registry initialized", "registered_types", providerRegistry.List())

	// P5-C Provider Factory：从 Connection/Credential 创建业务 Client
	// 优先使用 Connection-based 配置，其次使用 Legacy config.yaml
	providerFactory := providers.NewFactory(connectionManager, credentialService, &providers.LegacyConfig{
		ClusterConfigPath: cfg.Cluster.ConfigPath,
		Prometheus: providers.PrometheusLegacyConfig{
			Address: cfg.Prometheus.Address,
			Timeout: time.Duration(cfg.Prometheus.Timeout) * time.Second,
		},
		Elasticsearch: providers.ElasticsearchLegacyConfig{
			Address:  cfg.Elasticsearch.Address,
			Index:    cfg.Elasticsearch.Index,
			Username: cfg.Elasticsearch.Username,
			Password: cfg.Elasticsearch.Password,
			Timeout:  cfg.Elasticsearch.Timeout,
		},
		Jenkins: providers.JenkinsLegacyConfig{
			URL:      cfg.Jenkins.URL,
			Username: cfg.Jenkins.Username,
			Token:    cfg.Jenkins.Token,
			Timeout:  cfg.Jenkins.Timeout,
		},
		ArgoCD: providers.ArgoCDLegacyConfig{
			URL:     cfg.ArgoCD.URL,
			Token:   cfg.ArgoCD.Token,
			Timeout: cfg.ArgoCD.Timeout,
		},
	})

	// P5-C2: Prometheus Provider 迁移
	// 优先使用 Connection-based 配置创建 Querier，如果失败则保持 Legacy 配置
	if connQuerier, err := providerFactory.BuildPrometheusQuerier(context.Background(), rdb); err == nil && connQuerier != nil {
		querier = connQuerier
		if metricsHandler != nil {
			metricsHandler.Prom = querier
		}
		if anomalyService != nil {
			anomalyService.SetQuerier(querier)
		}
		slog.Info("prometheus querier migrated to provider factory")
	}

	// P5-C3: Elasticsearch Provider 迁移
	if connESClient, err := providerFactory.BuildElasticsearchClient(context.Background()); err == nil && connESClient != nil {
		esClient = connESClient
		slog.Info("elasticsearch client migrated to provider factory")
	}

	// P5-D: Kubernetes Provider 迁移
	// 优先使用 Connection-based 配置更新 cluster.Manager
	// 如果数据库中存在有效的 Kubernetes Connection，则使用 Connection 配置
	// 如果不存在，则保持 Legacy config.yaml 配置（向后兼容）
	if connClusters, err := providerFactory.BuildKubernetesClusters(context.Background()); err == nil && len(connClusters) > 0 {
		mgr.SetClusters(connClusters)
		slog.Info("kubernetes cluster manager migrated to provider factory", "clusters", len(connClusters))
	} else {
		slog.Info("kubernetes cluster manager using legacy config", "clusters", len(mgr.List()))
	}

	// Connection Handler
	connectionHandler := connection.NewHandler(connectionService, credentialService, connectionManager, providerRegistry)

	// P1-PRODUCT-05: Connection 周期健康检查器
	// 默认 5 分钟检查周期，15 秒单 Provider 超时，4 并发
	connectionHealthChecker := connection.NewHealthChecker(
		connectionRepo,
		providerRegistry,
		5*time.Minute,
		15*time.Second,
		4,
	)
	connectionHandler.SetHealthChecker(connectionHealthChecker)
	connectionHealthChecker.Start(context.Background())
	slog.Info("connection & credential manager initialized")

	clusterSvc := cluster.NewService(mgr)
	automationEngine := automation.NewEngine(clusterSvc)

	// P5-C4: Jenkins Provider 迁移
	jenkinsClient := automation.NewJenkinsClient(cfg.Jenkins.URL, cfg.Jenkins.Username, cfg.Jenkins.Token, cfg.Jenkins.Timeout)
	if connJenkinsClient, err := providerFactory.BuildJenkinsClient(context.Background()); err == nil && connJenkinsClient != nil {
		jenkinsClient = connJenkinsClient
		slog.Info("jenkins client migrated to provider factory")
	}

	// P5-C5: ArgoCD Provider 迁移
	argocdClient := automation.NewArgoCDClient(cfg.ArgoCD.URL, cfg.ArgoCD.Token, cfg.ArgoCD.Timeout)
	if connArgoCDClient, err := providerFactory.BuildArgoCDClient(context.Background()); err == nil && connArgoCDClient != nil {
		argocdClient = connArgoCDClient
		slog.Info("argocd client migrated to provider factory")
	}

	// LogsHandler 提前创建，用于 Connection 变更回调动态更新 ES client
	logsHandler := &handler.LogsHandler{ES: esClient, Analyzer: logAnalyzer}

	// P5-E: Connection 变更动态更新 Provider
	// 在 Connection 创建/更新/删除/启用/禁用后，自动重新构建对应 Provider，
	// 避免用户添加 Connection 后需要重启后端才能生效。
	connectionService.RegisterOnChanged(func(ctx context.Context, connType connection.ConnectionType) {
		switch connType {
		case connection.TypeKubernetes:
			if clusters, err := providerFactory.BuildKubernetesClusters(ctx); err == nil {
				if len(clusters) > 0 {
					mgr.SetClusters(clusters)
					slog.Info("kubernetes cluster manager refreshed by connection change", "clusters", len(clusters))
				} else {
					// 没有 K8s Connection 时，回退到 legacy config
					if legacyClusters, err := cluster.LoadRegistry(cfg.Cluster.ConfigPath); err == nil {
						mgr.SetClusters(legacyClusters)
						slog.Info("kubernetes cluster manager fell back to legacy config", "clusters", len(legacyClusters))
					}
				}
			} else {
				slog.Warn("failed to refresh kubernetes clusters on connection change", "error", err)
			}

		case connection.TypePrometheus:
			if newQuerier, err := providerFactory.BuildPrometheusQuerier(ctx, rdb); err == nil && newQuerier != nil {
				querier = newQuerier
				if metricsHandler != nil {
					metricsHandler.Prom = newQuerier
				}
				if anomalyService != nil {
					anomalyService.SetQuerier(newQuerier)
				}
				slog.Info("prometheus querier refreshed by connection change")
			}

		case connection.TypeElasticsearch:
			if newES, err := providerFactory.BuildElasticsearchClient(ctx); err == nil && newES != nil {
				esClient = newES
				logsHandler.ES = newES
				slog.Info("elasticsearch client refreshed by connection change")
			}

		case connection.TypeJenkins:
			if newJenkins, err := providerFactory.BuildJenkinsClient(ctx); err == nil && newJenkins != nil {
				jenkinsClient = newJenkins
				slog.Info("jenkins client refreshed by connection change")
			}

		case connection.TypeArgoCD:
			if newArgo, err := providerFactory.BuildArgoCDClient(ctx); err == nil && newArgo != nil {
				argocdClient = newArgo
				slog.Info("argocd client refreshed by connection change")
			}
		}
	})
	slog.Info("connection change callback registered for dynamic provider refresh")

	// Automation Action Framework（审批+执行+审计）。
	actionRepo := automation.NewActionRepository(db)
	executionRepo := automation.NewExecutionRepository(db)
	automationAuditRepo := automation.NewAuditRepository(db)
	automationPolicy := automation.NewPolicyEngine(cfg.Server.Mode)
	k8sExecutor := automation.NewKubernetesExecutor(clusterSvc)
	jenkinsExecutor := automation.NewJenkinsExecutor(jenkinsClient)
	jenkinsExecutor.SetResolver(providerFactory)
	argocdExecutor := automation.NewArgoCDExecutor(argocdClient)
	argocdExecutor.SetResolver(providerFactory)
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
	workflowService.SetK8sQuerier(clusterSvc)
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
		Service:       aiAnalysisService,
		Repository:    aiAnalysisRepo,
		Enabled:       cfg.AI.Enabled && aiProvider != nil,
		ActionCreator: automation.NewActionCreatorAdapter(automationService, incidentService),
	}

	// AI Tool Calling Engine（只读工具，依赖 RCA/Topology/Cluster 等服务）。
	var toolEngine *tools.Engine
	var toolAuditRepo *tools.ToolAuditRepository
	var convRepo *ai.ConversationRepository
	var convHandler *ai.ConversationHandler
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
		convRepo = ai.NewConversationRepository(db)
		convHandler = ai.NewConversationHandler(convRepo)
		slog.Info("ai tool calling engine initialized", "tools", len(toolRegistry.List()))
	}

	// 认证服务。
	userRepo := auth.NewRepository(db)
	authService := auth.NewService(userRepo, cfg.Auth.JWTSecret, time.Duration(cfg.Auth.JWTExpiration)*time.Hour)
	auditRepo := audit.NewRepository(db)
	rateLimiter := redisutil.NewRateLimiter(rdb, 100, time.Minute)

	// 检查数据库中是否已配置 API Key
	dbAPIKeyConfigured := false
	if openAIProvider != nil {
		if k, err := aiConfigRepo.GetAPIKey(context.Background()); err == nil && k != "" {
			dbAPIKeyConfigured = true
		}
	}
	aiHandler := &handler.AIHandler{Assistant: aiAssistant, Engine: toolEngine, AuditRepo: toolAuditRepo, ConversationHdl: convHandler, Enabled: cfg.AI.Enabled, APIKeyConfigured: cfg.AI.APIKey != "" || dbAPIKeyConfigured}
	// 设置配置更新回调：前端更新 API Key 后，通知 AIHandler 重新加载状态
	aiConfigHandler.OnUpdate = func() {
		if configured, err := aiConfigRepo.IsConfigured(context.Background()); err == nil {
			aiHandler.UpdateAPIKeyStatus(configured)
		}
	}

	// Multi-Agent Orchestration 初始化
	agentRegistry := agent.NewRegistry()
	if err := agent.RegisterBuiltinAgents(agentRegistry); err != nil {
		slog.Error("failed to register builtin agents", "error", err)
	}
	agentOrchestrator := agent.NewOrchestrator(agentRegistry)
	agentHandler := agent.NewHandler(agentOrchestrator, agentRegistry)
	slog.Info("multi-agent orchestration initialized", "agents", len(agentRegistry.GetAll()))

	router := api.NewRouter(cfg.Server.Mode, api.Deps{
		Health:     &handler.HealthHandler{DB: db, Redis: rdb},
		Cluster:    &handler.ClusterHandler{Service: clusterSvc},
		Metrics:    metricsHandler,
		Alert:      &handler.AlertHandler{Repo: alertRepo, Aggregator: alertAggregator, NoiseReducer: alertNoiseReducer, IncidentService: incidentService},
		Anomaly:    anomalyHandler,
		RCA:        &handler.RCAHandler{AlertRepo: alertRepo, Engine: rcaEngine},
		Logs:       logsHandler,
		AI:         aiHandler,
		AIConfig:   aiConfigHandler,
		AIConversation: convHandler,
		Agent:      agentHandler,
		Connection: connectionHandler,
		Search: &handler.SearchHandler{IncidentRepo: incidentRepo, AlertRepo: alertRepo, ClusterSvc: clusterSvc},
		Automation: &handler.AutomationHandler{Engine: automationEngine},
		AutomationAction: automationActionHandler,
		Workflow:        workflowHandler,
		Jenkins:    &handler.JenkinsHandler{Jenkins: jenkinsClient, Resolver: providerFactory},
		ArgoCD:     &handler.ArgoCDHandler{ArgoCD: argocdClient, Resolver: providerFactory},
		Auth:       &handler.AuthHandler{AuthService: authService, UserRepo: userRepo, AuditRepo: auditRepo},
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

	// P1-R1 / P2-02 Reliability: 服务启动时恢复因 Worker 崩溃而遗留的 RUNNING 状态 Action/Workflow。
	// 同步执行模型下，服务重启意味着之前的 execution goroutine 已消失，无安全 Resume 能力。
	// 因此启动时所有遗留 RUNNING 都视为 interrupted execution，统一标记为 TIMEOUT/FAILED。
	recoveryCtx, recoveryCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if recovered, err := automationService.RecoverStaleActions(recoveryCtx, 5*time.Minute); err != nil {
		slog.Error("recover stale actions failed", "error", err)
	} else if recovered > 0 {
		slog.Info("recovered stale actions on startup", "count", recovered)
	}
	if recovered, err := workflowService.RecoverStaleExecutions(recoveryCtx, 5*time.Minute); err != nil {
		slog.Error("recover stale workflows failed", "error", err)
	} else if recovered > 0 {
		slog.Info("recovered stale workflows on startup", "count", recovered)
	}
	recoveryCancel()

	// P2-02: 启动 Runtime Recovery Scanner，定期扫描 lease 已过期的 RUNNING 任务。
	// heartbeat 机制保证正常执行的任务 lease 持续刷新，不会被误恢复。
	scannerCtx, scannerCancel := context.WithCancel(context.Background())
	automationService.StartRecoveryScanner(scannerCtx)
	workflowService.StartRecoveryScanner(scannerCtx)

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

	// 停止 Runtime Recovery Scanner。
	scannerCancel()

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
	type metricQuery struct {
		name string
		unit string
		expr string
	}
	var queries []metricQuery

	if resourceType == "pod" {
		queries = []metricQuery{
			{
				name: "container_cpu_usage",
				unit: "cores",
				expr: fmt.Sprintf(`rate(container_cpu_usage_seconds_total{pod="%s",namespace="%s",container!="",container!="POD"}[5m])`, resourceName, namespace),
			},
			{
				name: "container_memory_working_set",
				unit: "bytes",
				expr: fmt.Sprintf(`container_memory_working_set_bytes{pod="%s",namespace="%s",container!="",container!="POD"}`, resourceName, namespace),
			},
			{
				name: "container_restart_count",
				unit: "count",
				expr: fmt.Sprintf(`kube_pod_container_status_restarts_total{pod="%s",namespace="%s"}`, resourceName, namespace),
			},
			{
				name: "up",
				unit: "status",
				expr: fmt.Sprintf(`up{pod="%s",namespace="%s"}`, resourceName, namespace),
			},
		}
	} else if resourceType == "node" {
		queries = []metricQuery{
			{
				name: "node_cpu_usage",
				unit: "percent",
				expr: fmt.Sprintf(`100 - (avg by(instance) (rate(node_cpu_seconds_total{mode="idle",instance=~"%s.*"}[5m])) * 100)`, resourceName),
			},
			{
				name: "node_memory_usage",
				unit: "percent",
				expr: fmt.Sprintf(`(1 - node_memory_MemAvailable_bytes{instance=~"%s.*"} / node_memory_MemTotal_bytes{instance=~"%s.*"}) * 100`, resourceName, resourceName),
			},
		}
	}

	// 时间范围：Before 30min ~ After 15min（如果 Incident 已结束）
	queryStart := since
	queryEnd := until
	if queryEnd.Before(queryStart) {
		queryEnd = time.Now().UTC()
	}
	// 扩展查询范围以包含 After 数据
	queryEndExtended := queryEnd.Add(15 * time.Minute)
	if queryEndExtended.After(time.Now().UTC().Add(5 * time.Minute)) {
		queryEndExtended = time.Now().UTC()
	}

	// 根据时间范围设置 step
	duration := queryEndExtended.Sub(queryStart)
	step := 30 * time.Second
	if duration > 2*time.Hour {
		step = 1 * time.Minute
	}
	if duration > 6*time.Hour {
		step = 5 * time.Minute
	}

	for _, q := range queries {
		result, err := c.querier.QueryRange(ctx, q.expr, queryStart, queryEndExtended, step)
		if err != nil {
			slog.Warn("rca: metric query_range failed", "metric", q.name, "err", err)
			continue
		}
		if result == nil {
			continue
		}

		// 解析 matrix 结果（时间序列）
		if matrix, ok := result.Result.(model.Matrix); ok {
			for _, sampleStream := range matrix {
				if len(sampleStream.Values) == 0 {
					continue
				}

				// 提取 labels
				labels := make(map[string]string)
				for k, v := range sampleStream.Metric {
					labels[string(k)] = string(v)
				}

				// 按 Incident 时间分段
				var before, during, after []rca.MetricDataPoint
				var latestValue float64
				var latestTimestamp time.Time

				for _, vp := range sampleStream.Values {
					ts := vp.Timestamp.Time()
					val := float64(vp.Value)
					dp := rca.MetricDataPoint{Timestamp: ts, Value: val}

					if ts.Before(since) {
						before = append(before, dp)
					} else if ts.After(until) && !until.IsZero() {
						after = append(after, dp)
					} else {
						during = append(during, dp)
					}

					if ts.After(latestTimestamp) {
						latestTimestamp = ts
						latestValue = val
					}
				}

				if latestTimestamp.IsZero() {
					continue
				}

				metrics = append(metrics, rca.MetricInfo{
					Metric:    q.name,
					Value:     latestValue,
					Timestamp: latestTimestamp,
					Resource:  resourceName,
					Unit:      q.unit,
					Labels:    labels,
					Before:    before,
					During:    during,
					After:     after,
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
	// Evidence 收集不限制日志级别，返回所有匹配的日志（INFO/WARN/ERROR 都可能包含根因线索）。
	result, err := c.esClient.Search(ctx, logging.SearchQuery{
		Namespace: namespace,
		Pod:       pod,
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

// CollectPodResourceState 收集 Pod 的实时 Kubernetes 资源状态。
func (c *rcaContextCollector) CollectPodResourceState(ctx context.Context, cluster, namespace, pod string) (*rca.PodResourceState, error) {
	if c.clusterSvc == nil || pod == "" {
		return nil, nil
	}
	p, err := c.clusterSvc.GetPod(ctx, cluster, namespace, pod)
	if err != nil {
		return nil, err
	}

	state := &rca.PodResourceState{
		Namespace:   namespace,
		Pod:         pod,
		Phase:       string(p.Status.Phase),
		NodeName:    p.Spec.NodeName,
		PodIP:       p.Status.PodIP,
		HostIP:      p.Status.HostIP,
		Containers:  make([]rca.PodContainerState, 0, len(p.Status.ContainerStatuses)),
		Conditions:  make([]rca.PodCondition, 0, len(p.Status.Conditions)),
	}

	if p.Status.StartTime != nil {
		state.StartTime = p.Status.StartTime.Time.Format(time.RFC3339)
	}

	// 计算总体 Ready 和总 RestartCount。
	totalRestart := int32(0)
	allReady := len(p.Status.ContainerStatuses) > 0
	for _, cs := range p.Status.ContainerStatuses {
		totalRestart += cs.RestartCount
		if !cs.Ready {
			allReady = false
		}
		containerState := rca.PodContainerState{
			Name:         cs.Name,
			Ready:        cs.Ready,
			RestartCount: cs.RestartCount,
		}
		// 当前状态。
		if cs.State.Running != nil {
			containerState.State = "running"
			containerState.StartedAt = cs.State.Running.StartedAt.Time.Format(time.RFC3339)
		} else if cs.State.Waiting != nil {
			containerState.State = "waiting"
			containerState.Reason = cs.State.Waiting.Reason
			containerState.Message = cs.State.Waiting.Message
		} else if cs.State.Terminated != nil {
			containerState.State = "terminated"
			containerState.Reason = cs.State.Terminated.Reason
			containerState.ExitCode = &cs.State.Terminated.ExitCode
			sig := cs.State.Terminated.Signal
			containerState.Signal = &sig
			if !cs.State.Terminated.StartedAt.IsZero() {
				containerState.StartedAt = cs.State.Terminated.StartedAt.Time.Format(time.RFC3339)
			}
			if !cs.State.Terminated.FinishedAt.IsZero() {
				containerState.FinishedAt = cs.State.Terminated.FinishedAt.Time.Format(time.RFC3339)
			}
		} else {
			containerState.State = "unknown"
		}
		// 上一次状态。
		if cs.LastTerminationState.Terminated != nil {
			containerState.LastState = "terminated"
			containerState.LastReason = cs.LastTerminationState.Terminated.Reason
			containerState.LastExitCode = &cs.LastTerminationState.Terminated.ExitCode
		} else if cs.LastTerminationState.Waiting != nil {
			containerState.LastState = "waiting"
			containerState.LastReason = cs.LastTerminationState.Waiting.Reason
		} else if cs.LastTerminationState.Running != nil {
			containerState.LastState = "running"
		}
		state.Containers = append(state.Containers, containerState)
	}
	state.Ready = allReady
	state.RestartCount = totalRestart

	// Conditions。
	for _, cond := range p.Status.Conditions {
		state.Conditions = append(state.Conditions, rca.PodCondition{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}

	return state, nil
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
				summary := ai.AlertSummary{
					Name:      s.Title,
					Severity:  s.Severity,
					Service:   s.Service,
					Pod:       s.ResourceName,
					Namespace: s.Namespace,
					StartsAt:  s.Timestamp,
				}
				summary.ID = ai.GenerateEvidenceID(incidentID, "alerts", "alert", s.ResourceName, s.Timestamp, s.Title)
				aiCtx.Alerts = append(aiCtx.Alerts, summary)
			}
		}
		aiCtx.DataSources.AlertsAvailable = len(aiCtx.Alerts) > 0
	}

	// 4. 获取 Anomalies。
	if p.anomalyRepo != nil && inc.ResourceName != "" {
		if anomalies, err := p.anomalyRepo.FindByResource(ctx, inc.Cluster, inc.ResourceType, inc.ResourceName, since); err == nil {
			for _, a := range anomalies {
				summary := ai.AnomalySummary{
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
				}
				summary.ID = ai.GenerateEvidenceID(incidentID, "anomalies", "anomaly", a.ResourceName, a.Timestamp, a.Metric)
				aiCtx.Anomalies = append(aiCtx.Anomalies, summary)
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
						summary := ai.MetricSummary{
							Metric:    name,
							Value:     float64(s.Value),
							Resource:  inc.ResourceName,
							Timestamp: s.Timestamp.Time(),
						}
						summary.ID = ai.GenerateEvidenceID(incidentID, "metrics", "metric", inc.ResourceName, s.Timestamp.Time(), name)
						aiCtx.Metrics = append(aiCtx.Metrics, summary)
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
				summary := ai.LogSummary{
					Timestamp: hit.Timestamp,
					Level:     hit.Level,
					Message:   hit.Message,
					Pod:       hit.Pod,
					Namespace: hit.Namespace,
				}
				summary.ID = ai.GenerateEvidenceID(incidentID, "logs", "log", hit.Pod, hit.Timestamp, hit.Message)
				aiCtx.Logs = append(aiCtx.Logs, summary)
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
			summary := ai.EventSummary{
				Type:         e.Type,
				Reason:       e.Reason,
				Message:      e.Message,
				ResourceType: inc.ResourceType,
				ResourceName: inc.ResourceName,
				Namespace:    inc.Namespace,
				Timestamp:    e.LastTimestamp.Time,
				Count:        e.Count,
			}
			summary.ID = ai.GenerateEvidenceID(incidentID, "events", "event", inc.ResourceName, e.LastTimestamp.Time, e.Reason)
			aiCtx.Events = append(aiCtx.Events, summary)
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

	// 9. 获取 Pod Diagnostic（Kubernetes 诊断信息）。
	if p.clusterSvc != nil && inc.ResourceType == "pod" && inc.ResourceName != "" {
		if pod, err := p.clusterSvc.GetPod(ctx, inc.Cluster, inc.Namespace, inc.ResourceName); err == nil && pod != nil {
			diag := &ai.PodDiagnosticSummary{
				Namespace:    inc.Namespace,
				Pod:          inc.ResourceName,
				PodUID:       string(pod.UID),
				Phase:        string(pod.Status.Phase),
				Ready:        isPodReady(pod),
				RestartCount: getPodRestartCount(pod),
				NodeName:     pod.Spec.NodeName,
			}
			if pod.Status.StartTime != nil {
				diag.StartTime = pod.Status.StartTime.Format(time.RFC3339)
			}
			for _, cs := range pod.Status.ContainerStatuses {
				containerDiag := ai.PodContainerDiagnostic{
					Name:         cs.Name,
					Ready:        cs.Ready,
					RestartCount: cs.RestartCount,
					State:        getContainerState(cs.State),
				}
				if cs.State.Waiting != nil {
					containerDiag.Reason = cs.State.Waiting.Reason
					containerDiag.Message = cs.State.Waiting.Message
				}
				if cs.State.Terminated != nil {
					containerDiag.Reason = cs.State.Terminated.Reason
					containerDiag.Message = cs.State.Terminated.Message
					containerDiag.ExitCode = &cs.State.Terminated.ExitCode
					if !cs.State.Terminated.StartedAt.IsZero() {
						containerDiag.StartedAt = cs.State.Terminated.StartedAt.Format(time.RFC3339)
					}
					if !cs.State.Terminated.FinishedAt.IsZero() {
						containerDiag.FinishedAt = cs.State.Terminated.FinishedAt.Format(time.RFC3339)
					}
				}
				if cs.State.Running != nil && !cs.State.Running.StartedAt.IsZero() {
					containerDiag.StartedAt = cs.State.Running.StartedAt.Format(time.RFC3339)
				}
				if cs.LastTerminationState.Terminated != nil {
					containerDiag.LastState = "terminated"
					containerDiag.LastReason = cs.LastTerminationState.Terminated.Reason
					containerDiag.LastExitCode = &cs.LastTerminationState.Terminated.ExitCode
					if !cs.LastTerminationState.Terminated.StartedAt.IsZero() {
						containerDiag.LastStartedAt = cs.LastTerminationState.Terminated.StartedAt.Format(time.RFC3339)
					}
					if !cs.LastTerminationState.Terminated.FinishedAt.IsZero() {
						containerDiag.LastFinishedAt = cs.LastTerminationState.Terminated.FinishedAt.Format(time.RFC3339)
					}
				} else if cs.LastTerminationState.Waiting != nil {
					containerDiag.LastState = "waiting"
					containerDiag.LastReason = cs.LastTerminationState.Waiting.Reason
				}
				diag.Containers = append(diag.Containers, containerDiag)
			}
			// 生成 Evidence ID（包含 PodUID 确保 Historical State 可关联）
			diag.ID = ai.GenerateEvidenceID(incidentID, "pod_diagnostic", "pod_state", inc.ResourceName, time.Now(), fmt.Sprintf("%s-%s-%d-%s", diag.PodUID, diag.Phase, diag.RestartCount, diag.NodeName))
			aiCtx.PodDiagnostic = diag
		}
	}

	// 10. 获取 Deployment Diagnostic（Kubernetes 诊断信息）。
	if p.clusterSvc != nil && inc.ResourceType == "deployment" && inc.ResourceName != "" {
		if dep, err := p.clusterSvc.GetDeployment(ctx, inc.Cluster, inc.Namespace, inc.ResourceName); err == nil && dep != nil {
			diag := &ai.DeploymentDiagnosticSummary{
				Namespace:           inc.Namespace,
				Deployment:          inc.ResourceName,
				Replicas:            *dep.Spec.Replicas,
				AvailableReplicas:   dep.Status.AvailableReplicas,
				ReadyReplicas:       dep.Status.ReadyReplicas,
				UpdatedReplicas:     dep.Status.UpdatedReplicas,
				UnavailableReplicas: dep.Status.UnavailableReplicas,
			}
			// 获取 Deployment condition
			for _, cond := range dep.Status.Conditions {
				if cond.Type == appsv1.DeploymentAvailable {
					diag.Condition = string(cond.Type)
					diag.ConditionReason = cond.Reason
					diag.ConditionMessage = cond.Message
					break
				}
			}
			// 生成 Evidence ID
			diag.ID = ai.GenerateEvidenceID(incidentID, "deployment_diagnostic", "deployment_state", inc.ResourceName, time.Now(), fmt.Sprintf("%d-%d-%d", diag.Replicas, diag.AvailableReplicas, diag.ReadyReplicas))
			aiCtx.DeploymentDiagnostic = diag
		}
	}

	// 11. 获取 Service Diagnostic（Kubernetes 诊断信息）。
	if p.clusterSvc != nil && inc.ResourceType == "service" && inc.ResourceName != "" {
		svc, err := p.clusterSvc.GetService(ctx, inc.Cluster, inc.Namespace, inc.ResourceName)
		if err == nil && svc != nil {
			diag := &ai.ServiceDiagnosticSummary{
				Namespace:   inc.Namespace,
				ServiceName: inc.ResourceName,
				ServiceType: string(svc.Spec.Type),
				ClusterIP:   svc.Spec.ClusterIP,
				Selector:    svc.Spec.Selector,
			}
			// 端口信息
			for _, port := range svc.Spec.Ports {
				diag.Ports = append(diag.Ports, ai.ServicePortInfo{
					Name:       port.Name,
					Port:       port.Port,
					TargetPort: port.TargetPort.String(),
					Protocol:   string(port.Protocol),
				})
			}
			// Endpoints 信息
			if ep, err := p.clusterSvc.GetEndpoints(ctx, inc.Cluster, inc.Namespace, inc.ResourceName); err == nil && ep != nil {
				for _, subset := range ep.Subsets {
					diag.EndpointCount += int32(len(subset.Addresses))
					diag.ReadyEndpointCount += int32(len(subset.Addresses))
					for _, addr := range subset.Addresses {
						diag.EndpointAddresses = append(diag.EndpointAddresses, addr.IP)
					}
					// NotReadyAddresses 不计入 ready
					diag.EndpointCount += int32(len(subset.NotReadyAddresses))
				}
			}
			// 通过 selector 匹配 Pod（内存匹配，避免 N+1 查询）
			if pods, err := p.clusterSvc.ListPods(ctx, inc.Cluster, inc.Namespace); err == nil {
				for i := range pods {
					pod := &pods[i]
					// 检查 selector 是否匹配
					matched := true
					for k, v := range svc.Spec.Selector {
						if pod.Labels[k] != v {
							matched = false
							break
						}
					}
					if matched {
						diag.MatchedPodCount++
						if isPodReady(pod) {
							diag.ReadyMatchedPodCount++
						}
					}
				}
			}
			// 判断 selector_match_status
			if diag.MatchedPodCount == 0 {
				diag.SelectorMatchStatus = "no_pods_matched"
			} else if diag.ReadyMatchedPodCount == 0 {
				diag.SelectorMatchStatus = "no_ready_pods"
			} else if diag.EndpointCount == 0 {
				diag.SelectorMatchStatus = "no_endpoints"
			} else {
				diag.SelectorMatchStatus = "matched"
			}
			// 生成 Evidence ID
			diag.ID = ai.GenerateEvidenceID(incidentID, "service_diagnostic", "service_state", inc.ResourceName, time.Now(), fmt.Sprintf("%s-%d-%d-%d", diag.SelectorMatchStatus, diag.MatchedPodCount, diag.ReadyMatchedPodCount, diag.EndpointCount))
			aiCtx.ServiceDiagnostic = diag
		}
	}

	return aiCtx, nil
}

// isPodReady 检查 Pod 是否 Ready。
func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// getPodRestartCount 获取 Pod 总重启次数。
func getPodRestartCount(pod *corev1.Pod) int32 {
	var total int32
	for _, cs := range pod.Status.ContainerStatuses {
		total += cs.RestartCount
	}
	return total
}

// getContainerState 获取容器状态字符串。
func getContainerState(state corev1.ContainerState) string {
	if state.Running != nil {
		return "running"
	}
	if state.Waiting != nil {
		return "waiting"
	}
	if state.Terminated != nil {
		return "terminated"
	}
	return "unknown"
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

package api

import (
	"net/http"

	"github.com/aiops/aiops-platform/internal/agent"
	"github.com/aiops/aiops-platform/internal/audit"
	"github.com/aiops/aiops-platform/internal/ai"
	"github.com/aiops/aiops-platform/internal/auth"
	"github.com/aiops/aiops-platform/internal/automation"
	"github.com/aiops/aiops-platform/internal/connection"
	"github.com/aiops/aiops-platform/internal/handler"
	"github.com/aiops/aiops-platform/internal/incident"
	"github.com/aiops/aiops-platform/internal/middleware"
	"github.com/aiops/aiops-platform/internal/redisutil"
	"github.com/aiops/aiops-platform/internal/topology"
	"github.com/aiops/aiops-platform/internal/workflow"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// maxRequestBodySize 限制 API 请求体大小为 10MB，防止恶意大请求导致内存耗尽。
const maxRequestBodySize = 10 << 20 // 10MB

type Deps struct {
	Health     *handler.HealthHandler
	Cluster    *handler.ClusterHandler
	Metrics    *handler.MetricsHandler
	Alert      *handler.AlertHandler
	Anomaly    *handler.AnomalyHandler
	RCA        *handler.RCAHandler
	Logs       *handler.LogsHandler
	AI         *handler.AIHandler
	AIConfig   *handler.AIConfigHandler
	AIConversation *ai.ConversationHandler
	Agent      *agent.Handler
	Connection *connection.Handler
	Search     *handler.SearchHandler
	Automation *handler.AutomationHandler
	AutomationAction *automation.Handler
	Workflow        *workflow.Handler
	Jenkins    *handler.JenkinsHandler
	ArgoCD     *handler.ArgoCDHandler
	Auth       *handler.AuthHandler
	AuthService *auth.Service
	Audit      *handler.AuditHandler
	Incident   *incident.Handler
	IncidentRCA *handler.IncidentRCAHandler
	IncidentAI  *ai.IncidentAIHandler
	Topology   *topology.Handler
	RateLimit  *redisutil.RateLimiter
}

func NewRouter(mode string, deps Deps) *gin.Engine {
	if mode == "" {
		mode = gin.DebugMode
	}
	gin.SetMode(mode)

	r := gin.New()
	r.Use(gin.Recovery())
	// RequestID 必须在 RequestLog 之前，这样日志中可以携带 request_id。
	r.Use(middleware.RequestID())
	r.Use(middleware.RequestLog())

	// /metrics 在 Metrics() 中间件之前注册，避免自统计。
	// promhttp.Handler() 会输出 Go runtime 指标（goroutine、内存、GC）+ 我们自定义的 HTTP 指标。
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.Use(middleware.Metrics())

	r.GET("/health", deps.Health.Health)
	r.GET("/ready", deps.Health.Ready)
	r.GET("/swagger.json", func(c *gin.Context) {
		c.File("docs/swagger.json")
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/swagger.json")))

	// P0-01: 公开端点（不需要认证）。
	// 必须在认证 v1 group 之前注册，避免被 AuthMiddleware 拦截。
	publicV1 := r.Group("/api/v1")
	{
		// 登录接口公开。
		if deps.Auth != nil {
			publicV1.POST("/auth/login", deps.Auth.Login)
		}
		// Alertmanager Webhook 公开（通过 shared secret 验证，由 handler 内部处理）。
		if deps.Alert != nil {
			publicV1.POST("/alerts/webhook", deps.Alert.ReceiveWebhook)
		}
	}

	v1 := r.Group("/api/v1")
	{
		// P0-01: 统一认证中间件。所有 /api/v1 端点（除上述公开端点）都必须认证。
		if deps.AuthService != nil {
			v1.Use(auth.AuthMiddleware(deps.AuthService))
		}

		// 请求体大小限制：防止恶意大请求导致内存耗尽。
		v1.Use(func(c *gin.Context) {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodySize)
			c.Next()
		})

		// API 限流中间件。
		if deps.RateLimit != nil {
			v1.Use(deps.RateLimit.Middleware())
		}

		// 审计中间件：自动记录所有写操作。
		if deps.Audit != nil && deps.Audit.Repo != nil {
			v1.Use(audit.Middleware(deps.Audit.Repo))
		}

		v1.GET("/clusters", deps.Cluster.ListClusters)
		v1.GET("/nodes", deps.Cluster.ListNodes)
		v1.GET("/nodes/:name", deps.Cluster.GetNode)
		v1.GET("/nodes/metrics", deps.Cluster.GetNodeMetrics)
		v1.GET("/namespaces", deps.Cluster.ListNamespaces)
		v1.GET("/pods", deps.Cluster.ListPods)
		v1.GET("/pods/:name", deps.Cluster.GetPod)
		v1.GET("/pods/metrics", deps.Cluster.GetPodMetrics)
		v1.GET("/deployments", deps.Cluster.ListDeployments)
		v1.GET("/deployments/:name", deps.Cluster.GetDeployment)
		v1.GET("/statefulsets", deps.Cluster.ListStatefulSets)
		v1.GET("/daemonsets", deps.Cluster.ListDaemonSets)
		v1.GET("/services", deps.Cluster.ListServices)
		v1.GET("/configmaps", deps.Cluster.ListConfigMaps)
		v1.GET("/secrets", deps.Cluster.ListSecrets)

		if deps.Metrics != nil {
			v1.GET("/metrics/query", deps.Metrics.Query)
			v1.GET("/metrics/range", deps.Metrics.QueryRange)
			v1.GET("/metrics/nodes", deps.Metrics.ListNodes)
			v1.GET("/metrics/pods", deps.Metrics.ListPods)
		}

		if deps.Alert != nil {
			// P0-01: webhook 已在 publicV1 注册，此处不重复注册。
			v1.GET("/alerts", deps.Alert.List)
			v1.GET("/alerts/aggregate", deps.Alert.Aggregate)
			v1.GET("/alerts/noise", deps.Alert.Reduce)
			v1.GET("/alerts/:id", deps.Alert.Get)
			v1.POST("/alerts/:id/acknowledge", auth.RequirePermission("alerts", "write"), deps.Alert.Acknowledge)
			v1.POST("/alerts/:id/resolve", auth.RequirePermission("alerts", "write"), deps.Alert.Resolve)
		}

		if deps.Anomaly != nil {
			v1.GET("/anomaly", deps.Anomaly.List)
			v1.GET("/anomaly/active/count", deps.Anomaly.ActiveCount)
			v1.GET("/anomaly/:id", deps.Anomaly.Get)
			v1.POST("/anomaly/detect", deps.Anomaly.Detect)
		}

		if deps.RCA != nil {
			v1.GET("/rca/analyze", deps.RCA.Analyze)
		}

		if deps.Logs != nil {
			v1.GET("/logs/search", deps.Logs.Search)
			v1.GET("/logs/analyze", deps.Logs.Analyze)
		}

		if deps.AI != nil {
			v1.POST("/ai/ask", deps.AI.Ask)
			v1.GET("/ai/audit", deps.AI.ListAudit)
		}

		if deps.AIConfig != nil {
			v1.GET("/ai/config", auth.RequirePermission("ai", "read"), deps.AIConfig.GetConfig)
			v1.POST("/ai/config", auth.RequirePermission("ai", "write"), deps.AIConfig.UpdateConfig)
		}

		if deps.AIConversation != nil {
			v1.GET("/ai/conversations", deps.AIConversation.ListConversations)
			v1.GET("/ai/conversations/:id", deps.AIConversation.GetConversation)
			v1.DELETE("/ai/conversations/:id", deps.AIConversation.DeleteConversation)
		}

		if deps.Agent != nil {
			v1.POST("/agent/orchestrate", deps.Agent.Orchestrate)
			v1.GET("/agent/results", deps.Agent.ListResults)
			v1.GET("/agent/results/:id", deps.Agent.GetResult)
			v1.GET("/agent/results/:id/events", deps.Agent.GetEvents)
			v1.POST("/agent/results/:id/approve", deps.Agent.Approve)
			v1.POST("/agent/results/:id/reject", deps.Agent.Reject)
			v1.GET("/agents", deps.Agent.ListAgents)
		}

		if deps.Connection != nil {
			// Connection CRUD
			v1.GET("/connections", deps.Connection.ListConnections)
			v1.GET("/connections/types", deps.Connection.ListConnectionTypes)
			v1.GET("/connections/:id", deps.Connection.GetConnection)
			v1.POST("/connections", auth.RequirePermission("connection", "write"), deps.Connection.CreateConnection)
			v1.PUT("/connections/:id", auth.RequirePermission("connection", "write"), deps.Connection.UpdateConnection)
			v1.DELETE("/connections/:id", auth.RequirePermission("connection", "write"), deps.Connection.DeleteConnection)
			v1.POST("/connections/:id/enable", auth.RequirePermission("connection", "write"), deps.Connection.EnableConnection)
			v1.POST("/connections/:id/disable", auth.RequirePermission("connection", "write"), deps.Connection.DisableConnection)
			v1.POST("/connections/:id/test", auth.RequirePermission("connection", "write"), deps.Connection.TestConnection)

			// Credential CRUD
			v1.GET("/credentials", deps.Connection.ListCredentials)
			v1.GET("/credentials/:id", deps.Connection.GetCredential)
			v1.POST("/credentials", auth.RequirePermission("credential", "write"), deps.Connection.CreateCredential)
			v1.PUT("/credentials/:id", auth.RequirePermission("credential", "write"), deps.Connection.UpdateCredential)
			v1.DELETE("/credentials/:id", auth.RequirePermission("credential", "write"), deps.Connection.DeleteCredential)
		}

		if deps.Search != nil {
			v1.GET("/search", deps.Search.Search)
		}

		if deps.Automation != nil {
			// 只读操作：不需要权限控制（但需要认证，v1 已统一）
			v1.GET("/automation/pods/:pod/logs", deps.Automation.GetPodLogs)
			v1.GET("/automation/pods/:pod/events", deps.Automation.GetPodEvents)

			// P0-01: 危险操作必须认证 + 权限控制（v1 已统一 AuthMiddleware）
			v1.POST("/automation/pods/:pod/restart", auth.RequirePermission("cluster", "write"), deps.Automation.RestartPod)
			v1.POST("/automation/pods/:pod/exec", auth.RequirePermission("cluster", "write"), deps.Automation.ExecPod)
			v1.POST("/automation/deployments/:name/scale", auth.RequirePermission("cluster", "write"), deps.Automation.ScaleDeployment)
		}

		if deps.AutomationAction != nil {
			// P0-01: v1 已统一 AuthMiddleware，此处直接注册端点。
			v1.POST("/actions", auth.RequirePermission("automation", "create"), deps.AutomationAction.Create)
			v1.GET("/actions", deps.AutomationAction.List)
			v1.GET("/actions/pending-approval", deps.AutomationAction.PendingApproval)
			v1.GET("/actions/:id", deps.AutomationAction.Get)
			v1.POST("/actions/:id/approve", auth.RequirePermission("automation", "approve"), deps.AutomationAction.Approve)
			v1.POST("/actions/:id/reject", auth.RequirePermission("automation", "approve"), deps.AutomationAction.Reject)
			v1.POST("/actions/:id/dry-run", deps.AutomationAction.DryRun)
			v1.POST("/actions/:id/execute", auth.RequirePermission("automation", "execute"), deps.AutomationAction.Execute)
			v1.POST("/actions/:id/cancel", auth.RequirePermission("automation", "cancel"), deps.AutomationAction.Cancel)
			v1.GET("/actions/:id/executions", deps.AutomationAction.Executions)
			v1.GET("/automation/audit", auth.RequirePermission("automation", "audit"), deps.AutomationAction.Audit)
		}

		if deps.Workflow != nil {
			// P0-01: v1 已统一 AuthMiddleware，此处直接注册端点并添加权限控制。
			v1.POST("/workflows", auth.RequirePermission("automation", "create"), deps.Workflow.Create)
			v1.GET("/workflows", deps.Workflow.List)
			v1.GET("/workflows/:id", deps.Workflow.Get)
			v1.POST("/workflows/:id/submit", auth.RequirePermission("automation", "create"), deps.Workflow.Submit)
			v1.POST("/workflows/:id/approve", auth.RequirePermission("automation", "approve"), deps.Workflow.Approve)
			v1.POST("/workflows/:id/execute", auth.RequirePermission("automation", "execute"), deps.Workflow.Execute)
			v1.POST("/workflows/:id/cancel", auth.RequirePermission("automation", "cancel"), deps.Workflow.Cancel)
			v1.POST("/workflows/:id/dry-run", deps.Workflow.DryRun)
			v1.GET("/workflows/:id/executions", deps.Workflow.ListExecutions)
			v1.GET("/workflow-executions/:id", deps.Workflow.GetExecution)
		}

		if deps.Jenkins != nil {
			v1.GET("/jenkins/jobs", deps.Jenkins.ListJobs)
			v1.GET("/jenkins/jobs/:name/builds", deps.Jenkins.ListBuilds)
			v1.POST("/jenkins/jobs/:name/build", auth.RequirePermission("jenkins", "write"), deps.Jenkins.Build)
			v1.GET("/jenkins/jobs/:name/builds/:number/log", deps.Jenkins.GetBuildLog)
		}

		if deps.ArgoCD != nil {
			v1.GET("/argocd/apps", deps.ArgoCD.ListApps)
			v1.GET("/argocd/apps/:name", deps.ArgoCD.GetApp)
			v1.POST("/argocd/apps/:name/sync", auth.RequirePermission("argocd", "write"), deps.ArgoCD.SyncApp)
			v1.POST("/argocd/apps/:name/refresh", auth.RequirePermission("argocd", "write"), deps.ArgoCD.RefreshApp)
		}

		if deps.Auth != nil {
			// P0-01: 登录接口已在 publicV1 注册，此处不重复注册。
			v1.POST("/auth/logout", deps.Auth.Logout)
			v1.GET("/auth/me", deps.Auth.Me)
			v1.GET("/users", deps.Auth.ListUsers)
			v1.POST("/users", auth.RequirePermission("users", "write"), deps.Auth.CreateUser)
			v1.PUT("/users/:id", auth.RequirePermission("users", "write"), deps.Auth.UpdateUser)
			v1.PUT("/users/:id/status", auth.RequirePermission("users", "write"), deps.Auth.UpdateUserStatus)
			v1.PUT("/users/:id/password", auth.RequirePermission("users", "write"), deps.Auth.ResetPassword)
			v1.PUT("/users/:id/roles", auth.RequirePermission("users", "write"), deps.Auth.AssignRoles)
			v1.GET("/roles", deps.Auth.ListRoles)
		}

		if deps.Audit != nil {
			v1.GET("/audit-logs", auth.RequirePermission("audit", "read"), deps.Audit.List)
		}

		if deps.Incident != nil {
			v1.GET("/incidents", deps.Incident.List)
			v1.GET("/incidents/:id", deps.Incident.Get)
			v1.POST("/incidents/:id/acknowledge", auth.RequirePermission("incidents", "write"), deps.Incident.Acknowledge)
			v1.POST("/incidents/:id/resolve", auth.RequirePermission("incidents", "write"), deps.Incident.Resolve)
			v1.POST("/incidents/:id/close", auth.RequirePermission("incidents", "write"), deps.Incident.Close)
			v1.GET("/incidents/:id/signals", deps.Incident.Signals)
			v1.GET("/incidents/:id/timeline", deps.Incident.Timeline)
			if deps.AutomationAction != nil {
				// P0-01: v1 已统一 AuthMiddleware
				v1.POST("/incidents/:id/actions", auth.RequirePermission("automation", "create"), deps.AutomationAction.CreateFromIncident)
			}
			if deps.IncidentRCA != nil {
				v1.POST("/incidents/:id/rca", deps.IncidentRCA.Analyze)
				v1.GET("/incidents/:id/rca", deps.IncidentRCA.GetLatest)
				v1.GET("/incidents/:id/rca/history", deps.IncidentRCA.GetHistory)
				v1.POST("/incidents/:id/rca/reanalyze", deps.IncidentRCA.Reanalyze)
				v1.GET("/incidents/:id/evidence", deps.IncidentRCA.GetEvidence)
			}
			if deps.IncidentAI != nil {
				// P0-01: v1 已统一 AuthMiddleware
				v1.POST("/incidents/:id/ai-analyze", deps.IncidentAI.Analyze)
				v1.GET("/incidents/:id/ai-analysis", deps.IncidentAI.GetLatest)
				v1.GET("/incidents/:id/ai-analysis/history", deps.IncidentAI.GetHistory)
			}
		}

		if deps.Topology != nil {
			v1.GET("/topology", deps.Topology.GetGraph)
			v1.GET("/topology/nodes/:type/:name", deps.Topology.GetNode)
			v1.GET("/topology/dependencies/:type/:name", deps.Topology.GetDependencies)
			v1.GET("/topology/impact/:type/:name", deps.Topology.GetImpact)
			v1.POST("/topology/cache/invalidate", auth.RequirePermission("topology", "write"), deps.Topology.InvalidateCache)
		}
	}

	return r
}

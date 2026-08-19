package api

import (
	"github.com/aiops/aiops-platform/internal/audit"
	"github.com/aiops/aiops-platform/internal/ai"
	"github.com/aiops/aiops-platform/internal/handler"
	"github.com/aiops/aiops-platform/internal/incident"
	"github.com/aiops/aiops-platform/internal/middleware"
	"github.com/aiops/aiops-platform/internal/redisutil"
	"github.com/aiops/aiops-platform/internal/topology"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

type Deps struct {
	Health     *handler.HealthHandler
	Cluster    *handler.ClusterHandler
	Metrics    *handler.MetricsHandler
	Alert      *handler.AlertHandler
	Anomaly    *handler.AnomalyHandler
	RCA        *handler.RCAHandler
	Logs       *handler.LogsHandler
	AI         *handler.AIHandler
	Automation *handler.AutomationHandler
	Jenkins    *handler.JenkinsHandler
	ArgoCD     *handler.ArgoCDHandler
	Auth       *handler.AuthHandler
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

	v1 := r.Group("/api/v1")
	{
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
		v1.GET("/namespaces", deps.Cluster.ListNamespaces)
		v1.GET("/pods", deps.Cluster.ListPods)
		v1.GET("/pods/:name", deps.Cluster.GetPod)
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
			v1.POST("/alerts/webhook", deps.Alert.ReceiveWebhook)
			v1.GET("/alerts", deps.Alert.List)
			v1.GET("/alerts/aggregate", deps.Alert.Aggregate)
			v1.GET("/alerts/noise", deps.Alert.Reduce)
			v1.GET("/alerts/:id", deps.Alert.Get)
			v1.POST("/alerts/:id/acknowledge", deps.Alert.Acknowledge)
			v1.POST("/alerts/:id/resolve", deps.Alert.Resolve)
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

		if deps.Automation != nil {
			v1.GET("/automation/pods/:pod/logs", deps.Automation.GetPodLogs)
			v1.GET("/automation/pods/:pod/events", deps.Automation.GetPodEvents)
			v1.POST("/automation/pods/:pod/restart", deps.Automation.RestartPod)
			v1.POST("/automation/deployments/:name/scale", deps.Automation.ScaleDeployment)
		}

		if deps.Jenkins != nil {
			v1.GET("/jenkins/jobs", deps.Jenkins.ListJobs)
			v1.GET("/jenkins/jobs/:name/builds", deps.Jenkins.ListBuilds)
			v1.POST("/jenkins/jobs/:name/build", deps.Jenkins.Build)
			v1.GET("/jenkins/jobs/:name/builds/:number/log", deps.Jenkins.GetBuildLog)
		}

		if deps.ArgoCD != nil {
			v1.GET("/argocd/apps", deps.ArgoCD.ListApps)
			v1.GET("/argocd/apps/:name", deps.ArgoCD.GetApp)
			v1.POST("/argocd/apps/:name/sync", deps.ArgoCD.SyncApp)
			v1.POST("/argocd/apps/:name/refresh", deps.ArgoCD.RefreshApp)
		}

		if deps.Auth != nil {
			// 登录接口公开。
			v1.POST("/auth/login", deps.Auth.Login)
			v1.POST("/auth/logout", deps.Auth.Logout)
			v1.GET("/auth/me", deps.Auth.Me)
			v1.GET("/users", deps.Auth.ListUsers)
			v1.POST("/users", deps.Auth.CreateUser)
			v1.GET("/roles", deps.Auth.ListRoles)
		}

		if deps.Audit != nil {
			v1.GET("/audit-logs", deps.Audit.List)
		}

		if deps.Incident != nil {
			v1.GET("/incidents", deps.Incident.List)
			v1.GET("/incidents/:id", deps.Incident.Get)
			v1.POST("/incidents/:id/acknowledge", deps.Incident.Acknowledge)
			v1.POST("/incidents/:id/resolve", deps.Incident.Resolve)
			v1.POST("/incidents/:id/close", deps.Incident.Close)
			v1.GET("/incidents/:id/signals", deps.Incident.Signals)
			v1.GET("/incidents/:id/timeline", deps.Incident.Timeline)
			if deps.IncidentRCA != nil {
				v1.POST("/incidents/:id/rca", deps.IncidentRCA.Analyze)
				v1.GET("/incidents/:id/rca", deps.IncidentRCA.GetLatest)
				v1.GET("/incidents/:id/rca/history", deps.IncidentRCA.GetHistory)
				v1.POST("/incidents/:id/rca/reanalyze", deps.IncidentRCA.Reanalyze)
				v1.GET("/incidents/:id/evidence", deps.IncidentRCA.GetEvidence)
			}
			if deps.IncidentAI != nil {
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
			v1.POST("/topology/cache/invalidate", deps.Topology.InvalidateCache)
		}
	}

	return r
}

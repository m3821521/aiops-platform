package api

import (
	"github.com/aiops/aiops-platform/internal/handler"
	"github.com/aiops/aiops-platform/internal/middleware"
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
		v1.GET("/clusters", deps.Cluster.ListClusters)
		v1.GET("/nodes", deps.Cluster.ListNodes)
		v1.GET("/namespaces", deps.Cluster.ListNamespaces)
		v1.GET("/pods", deps.Cluster.ListPods)
		v1.GET("/deployments", deps.Cluster.ListDeployments)
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
	}

	return r
}

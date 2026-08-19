package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aiops/aiops-platform/internal/ai"
	"github.com/aiops/aiops-platform/internal/alert"
	"github.com/aiops/aiops-platform/internal/anomaly"
	"github.com/aiops/aiops-platform/internal/api"
	"github.com/aiops/aiops-platform/internal/auth"
	"github.com/aiops/aiops-platform/internal/automation"
	"github.com/aiops/aiops-platform/internal/cluster"
	"github.com/aiops/aiops-platform/internal/config"
	"github.com/aiops/aiops-platform/internal/handler"
	"github.com/aiops/aiops-platform/internal/infra"
	"github.com/aiops/aiops-platform/internal/logging"
	"github.com/aiops/aiops-platform/internal/monitoring"
	"github.com/aiops/aiops-platform/internal/rca"
	"github.com/aiops/aiops-platform/pkg/logger"
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
	promClient, err := monitoring.NewClient(cfg.Prometheus.Address, time.Duration(cfg.Prometheus.Timeout)*time.Second)
	if err != nil {
		slog.Warn("prometheus client disabled", "err", err)
	} else {
		querier := monitoring.NewCachedQuerier(promClient, rdb)
		metricsHandler = &handler.MetricsHandler{Prom: querier}
		anomalyHandler = &handler.AnomalyHandler{Service: anomaly.NewService(querier)}
		slog.Info("prometheus connected", "addr", cfg.Prometheus.Address)
	}

	alertRepo := alert.NewRepository(db)
	alertAggregator := alert.NewAggregator(alertRepo)
	alertNoiseReducer := alert.NewNoiseReducer(alertRepo)
	rcaEngine := rca.NewEngine()
	esClient := logging.NewClient(cfg.Elasticsearch.Address, cfg.Elasticsearch.Index,
		cfg.Elasticsearch.Username, cfg.Elasticsearch.Password, cfg.Elasticsearch.Timeout)
	logAnalyzer := logging.NewAnalyzer(10, 1*time.Hour)

	// AI 助手（可选）。
	var aiAssistant *ai.Assistant
	if cfg.AI.Enabled {
		aiProvider := ai.NewOpenAIProvider(cfg.AI.BaseURL, cfg.AI.APIKey, cfg.AI.Model, cfg.AI.Timeout)
		aiAssistant = ai.NewAssistant(aiProvider, alertRepo)
		slog.Info("AI assistant enabled", "provider", cfg.AI.Provider, "model", cfg.AI.Model)
	} else {
		slog.Info("AI assistant disabled")
	}

	clusterSvc := cluster.NewService(mgr)
	automationEngine := automation.NewEngine(clusterSvc)
	jenkinsClient := automation.NewJenkinsClient(cfg.Jenkins.URL, cfg.Jenkins.Username, cfg.Jenkins.Token, cfg.Jenkins.Timeout)
	argocdClient := automation.NewArgoCDClient(cfg.ArgoCD.URL, cfg.ArgoCD.Token, cfg.ArgoCD.Timeout)

	// 认证服务。
	userRepo := auth.NewRepository(db)
	authService := auth.NewService(userRepo, cfg.Auth.JWTSecret, time.Duration(cfg.Auth.JWTExpiration)*time.Hour)

	router := api.NewRouter(cfg.Server.Mode, api.Deps{
		Health:     &handler.HealthHandler{DB: db, Redis: rdb},
		Cluster:    &handler.ClusterHandler{Service: clusterSvc},
		Metrics:    metricsHandler,
		Alert:      &handler.AlertHandler{Repo: alertRepo, Aggregator: alertAggregator, NoiseReducer: alertNoiseReducer},
		Anomaly:    anomalyHandler,
		RCA:        &handler.RCAHandler{AlertRepo: alertRepo, Engine: rcaEngine},
		Logs:       &handler.LogsHandler{ES: esClient, Analyzer: logAnalyzer},
		AI:         &handler.AIHandler{Assistant: aiAssistant, Enabled: cfg.AI.Enabled},
		Automation: &handler.AutomationHandler{Engine: automationEngine},
		Jenkins:    &handler.JenkinsHandler{Jenkins: jenkinsClient},
		ArgoCD:     &handler.ArgoCDHandler{ArgoCD: argocdClient},
		Auth:       &handler.AuthHandler{AuthService: authService, UserRepo: userRepo},
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
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	_ = rdb.Close()

	slog.Info("aiops-platform stopped")
}

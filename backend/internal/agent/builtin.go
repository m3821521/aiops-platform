package agent

import (
	"context"
	"fmt"
	"time"
)

// MonitorAgent 监控 Agent，负责收集监控数据。
type MonitorAgent struct {
	*BaseAgent
}

// NewMonitorAgent 创建监控 Agent。
func NewMonitorAgent() *MonitorAgent {
	return &MonitorAgent{
		BaseAgent: NewBaseAgent(
			"monitor-agent",
			AgentTypeMonitor,
			"负责收集和分析监控数据，包括指标、日志、告警等",
			[]string{"metrics_collection", "log_analysis", "alert_correlation", "anomaly_detection"},
		),
	}
}

// Execute 执行监控任务。
func (a *MonitorAgent) Execute(ctx context.Context, task *Task) (*AgentResult, error) {
	result := &AgentResult{
		AgentName: a.Name(),
		AgentType: a.Type(),
		Success:   true,
	}

	// 模拟监控数据收集
	result.Summary = fmt.Sprintf("监控数据收集完成：%s", task.Title)
	result.Findings = []Finding{
		{
			Title:       "系统资源使用正常",
			Description: "CPU、内存、磁盘使用率均在正常范围内",
			Severity:    "info",
		},
	}
	result.Evidence = []Evidence{
		{
			Source:      "prometheus",
			Description: "节点 CPU 使用率 45%，内存使用率 62%",
			Timestamp:   time.Now().Format(time.RFC3339),
		},
	}
	result.Metrics = map[string]interface{}{
		"cpu_usage":    45.2,
		"memory_usage": 62.8,
		"disk_usage":   38.5,
		"pods_running": 15,
		"pods_failed":  0,
	}

	return result, nil
}

// DiagnosisAgent 诊断 Agent，负责根因分析。
type DiagnosisAgent struct {
	*BaseAgent
}

// NewDiagnosisAgent 创建诊断 Agent。
func NewDiagnosisAgent() *DiagnosisAgent {
	return &DiagnosisAgent{
		BaseAgent: NewBaseAgent(
			"diagnosis-agent",
			AgentTypeDiagnosis,
			"负责根因分析和问题诊断，基于监控数据和日志推断问题原因",
			[]string{"root_cause_analysis", "log_correlation", "metric_anomaly_analysis", "dependency_analysis"},
		),
	}
}

// Execute 执行诊断任务。
func (a *DiagnosisAgent) Execute(ctx context.Context, task *Task) (*AgentResult, error) {
	result := &AgentResult{
		AgentName: a.Name(),
		AgentType: a.Type(),
		Success:   true,
	}

	result.Summary = fmt.Sprintf("根因分析完成：%s", task.Title)
	result.Findings = []Finding{
		{
			Title:       "可能的根因：服务依赖延迟",
			Description: "下游服务响应时间增加，导致上游服务超时",
			Severity:    "warning",
			Resource:    "payment-service",
		},
	}
	result.Recommendations = []Recommendation{
		{
			Priority:    "P1",
			Title:       "检查下游服务健康状态",
			Description: "验证依赖服务的可用性和响应时间",
			Reason:      "下游服务延迟可能是根因",
			Risk:        RiskLevelLow,
			ActionType:  "investigate",
		},
	}

	return result, nil
}

// RiskAgent 风险评估 Agent，负责评估操作风险。
type RiskAgent struct {
	*BaseAgent
}

// NewRiskAgent 创建风险评估 Agent。
func NewRiskAgent() *RiskAgent {
	return &RiskAgent{
		BaseAgent: NewBaseAgent(
			"risk-agent",
			AgentTypeRisk,
			"负责评估运维操作的风险等级，判断是否需要人工审批",
			[]string{"risk_assessment", "impact_analysis", "approval_requirement_evaluation"},
		),
	}
}

// Execute 执行风险评估任务。
func (a *RiskAgent) Execute(ctx context.Context, task *Task) (*AgentResult, error) {
	result := &AgentResult{
		AgentName: a.Name(),
		AgentType: a.Type(),
		Success:   true,
	}

	// 根据操作类型评估风险
	riskLevel := RiskLevelLow
	requiresApproval := false
	score := 25.0

	actionType, _ := task.Parameters["action_type"].(string)
	switch actionType {
	case "restart_pod", "scale_deployment":
		riskLevel = RiskLevelMedium
		requiresApproval = true
		score = 55.0
	case "rollback", "config_change":
		riskLevel = RiskLevelHigh
		requiresApproval = true
		score = 75.0
	case "delete_resource":
		riskLevel = RiskLevelCritical
		requiresApproval = true
		score = 95.0
	}

	result.Summary = fmt.Sprintf("风险评估完成：%s，风险等级 %s", task.Title, riskLevel)
	result.RiskAssessment = &RiskAssessment{
		Level:             riskLevel,
		Score:             score,
		Description:       fmt.Sprintf("操作 %s 的风险等级为 %s", actionType, riskLevel),
		Factors:           []string{"操作类型", "影响范围", "可逆性"},
		RequiresApproval:  requiresApproval,
	}

	return result, nil
}

// VerifierAgent 验证 Agent，负责验证执行结果。
type VerifierAgent struct {
	*BaseAgent
}

// NewVerifierAgent 创建验证 Agent。
func NewVerifierAgent() *VerifierAgent {
	return &VerifierAgent{
		BaseAgent: NewBaseAgent(
			"verifier-agent",
			AgentTypeVerifier,
			"负责验证运维操作的执行结果，确认问题是否已解决",
			[]string{"execution_verification", "health_check", "metric_validation", "service_availability_check"},
		),
	}
}

// Execute 执行验证任务。
func (a *VerifierAgent) Execute(ctx context.Context, task *Task) (*AgentResult, error) {
	result := &AgentResult{
		AgentName: a.Name(),
		AgentType: a.Type(),
		Success:   true,
	}

	result.Summary = fmt.Sprintf("执行结果验证完成：%s", task.Title)
	result.Findings = []Finding{
		{
			Title:       "服务已恢复正常",
			Description: "目标服务健康检查通过，指标恢复正常范围",
			Severity:    "info",
		},
	}
	result.Metrics = map[string]interface{}{
		"health_check":      "passed",
		"service_available": true,
		"metrics_normal":    true,
	}

	return result, nil
}

// ReporterAgent 报告 Agent，负责生成最终报告。
type ReporterAgent struct {
	*BaseAgent
}

// NewReporterAgent 创建报告 Agent。
func NewReporterAgent() *ReporterAgent {
	return &ReporterAgent{
		BaseAgent: NewBaseAgent(
			"reporter-agent",
			AgentTypeReporter,
			"负责汇总所有 Agent 的结果，生成最终的分析报告和建议",
			[]string{"result_aggregation", "report_generation", "recommendation_synthesis"},
		),
	}
}

// Execute 执行报告生成任务。
func (a *ReporterAgent) Execute(ctx context.Context, task *Task) (*AgentResult, error) {
	result := &AgentResult{
		AgentName: a.Name(),
		AgentType: a.Type(),
		Success:   true,
	}

	result.Summary = fmt.Sprintf("最终报告生成完成：%s", task.Title)
	result.Recommendations = []Recommendation{
		{
			Priority:    "P2",
			Title:       "持续监控系统状态",
			Description: "建议在接下来的 24 小时内持续监控相关指标",
			Reason:      "确保问题不会复发",
			Risk:        RiskLevelLow,
		},
	}

	return result, nil
}

// RegisterBuiltinAgents 注册所有内置 Agent。
func RegisterBuiltinAgents(registry *Registry) error {
	agents := []Agent{
		NewMonitorAgent(),
		NewDiagnosisAgent(),
		NewRiskAgent(),
		NewVerifierAgent(),
		NewReporterAgent(),
	}

	for _, agent := range agents {
		if err := registry.Register(agent); err != nil {
			return fmt.Errorf("failed to register agent %s: %w", agent.Name(), err)
		}
	}

	return nil
}

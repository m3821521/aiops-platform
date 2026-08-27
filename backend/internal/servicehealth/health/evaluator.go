package health

import (
	"fmt"
	"strings"
	"time"

	"github.com/aiops/aiops-platform/internal/rca"
)

// DefaultEvaluator 是默认的 Health Evaluation Engine 实现。
//
// 评估流程：
//  1. 过滤出 fresh evidence（只有 fresh 可作为主要判断依据）
//  2. 如果没有 fresh evidence => Unknown
//  3. 评估 workload availability（deployment/statefulset/daemonset）
//  4. 评估 Pod 级异常（crashloop/oomkilled/image_pull/not_ready/restart）
//  5. 合并结果：Down > Degraded > Healthy
//  6. 生成可解释的 Reason
//
// 纯函数：不修改输入 Evidence，不产生副作用。
type DefaultEvaluator struct{}

// NewDefaultEvaluator 创建默认 Health Evaluator。
func NewDefaultEvaluator() *DefaultEvaluator {
	return &DefaultEvaluator{}
}

// Evaluate 执行 Health Evaluation。
func (e *DefaultEvaluator) Evaluate(evidences []rca.Evidence) HealthEvaluation {
	evaluatedAt := time.Now()

	// 1. 过滤 fresh evidence
	freshEvidences := filterFresh(evidences)

	// 2. 没有 fresh evidence => Unknown
	if len(freshEvidences) == 0 {
		return HealthEvaluation{
			State:       HealthStateUnknown,
			Reason:      reasonForUnknown(evidences),
			EvidenceIDs: collectAllIDs(evidences),
			EvaluatedAt: evaluatedAt,
		}
	}

	// 3. 评估 availability
	availState, availIDs := evaluateAvailability(freshEvidences)

	// 4. 评估 Pod 级异常
	podState, podIDs := evaluatePodAbnormal(freshEvidences)

	// 5. 合并结果（Down > Degraded > Healthy）
	finalState := mergeState(availState, podState)

	// 如果合并后仍是 Unknown（有 fresh evidence 但都不匹配规则），视为 Healthy
	// 因为有 fresh evidence 存在且没有异常信号
	if finalState == HealthStateUnknown {
		finalState = HealthStateHealthy
	}

	// 6. 收集所有相关 Evidence IDs
	allIDs := make([]string, 0, len(availIDs)+len(podIDs))
	allIDs = append(allIDs, availIDs...)
	allIDs = append(allIDs, podIDs...)
	// 如果没有匹配到具体规则的 evidence，包含所有 fresh evidence IDs
	if len(allIDs) == 0 {
		allIDs = collectAllIDs(freshEvidences)
	}

	return HealthEvaluation{
		State:       finalState,
		Reason:      reasonForState(finalState, availState, podState, freshEvidences),
		EvidenceIDs: allIDs,
		EvaluatedAt: evaluatedAt,
	}
}

// filterFresh 过滤出 TrustStatus=fresh 的 Evidence。
func filterFresh(evidences []rca.Evidence) []rca.Evidence {
	fresh := make([]rca.Evidence, 0, len(evidences))
	for _, e := range evidences {
		if isFresh(e) {
			fresh = append(fresh, e)
		}
	}
	return fresh
}

// collectAllIDs 收集所有 Evidence 的 ID。
func collectAllIDs(evidences []rca.Evidence) []string {
	ids := make([]string, 0, len(evidences))
	for _, e := range evidences {
		if e.ID != "" {
			ids = append(ids, e.ID)
		}
	}
	return ids
}

// reasonForUnknown 生成 Unknown 状态的 Reason。
// 分析为什么无法判断：只有 stale / error / empty / 无 evidence。
func reasonForUnknown(evidences []rca.Evidence) string {
	if len(evidences) == 0 {
		return "no evidence available"
	}

	staleCount := 0
	errorCount := 0
	emptyCount := 0
	otherCount := 0

	for _, e := range evidences {
		switch e.TrustStatus {
		case "stale":
			staleCount++
		case "error":
			errorCount++
		case "empty":
			emptyCount++
		default:
			otherCount++
		}
	}

	parts := make([]string, 0, 4)
	if staleCount > 0 {
		parts = append(parts, fmt.Sprintf("%d stale", staleCount))
	}
	if errorCount > 0 {
		parts = append(parts, fmt.Sprintf("%d error", errorCount))
	}
	if emptyCount > 0 {
		parts = append(parts, fmt.Sprintf("%d empty", emptyCount))
	}
	if otherCount > 0 {
		parts = append(parts, fmt.Sprintf("%d other", otherCount))
	}

	if len(parts) == 0 {
		return "no fresh evidence available"
	}
	return "no fresh evidence: " + strings.Join(parts, ", ")
}

// reasonForState 生成指定状态的 Reason。
func reasonForState(state, availState, podState HealthState, freshEvidences []rca.Evidence) string {
	switch state {
	case HealthStateDown:
		return reasonForDown(freshEvidences)
	case HealthStateDegraded:
		return reasonForDegraded(availState, podState, freshEvidences)
	case HealthStateHealthy:
		return reasonForHealthy(freshEvidences)
	default:
		return "unknown health state"
	}
}

// reasonForDown 生成 Down 状态的 Reason。
func reasonForDown(evidences []rca.Evidence) string {
	for _, e := range evidences {
		if !isAvailabilitySignal(e) {
			continue
		}
		avail := parseAvailability(e.Expected)
		if avail.valid && avail.desired > 0 && avail.ready == 0 {
			return fmt.Sprintf("%s availability: 0/%d replicas ready",
				strings.TrimSuffix(string(e.Type), "_availability"),
				avail.desired)
		}
	}
	return "service is down"
}

// reasonForDegraded 生成 Degraded 状态的 Reason。
func reasonForDegraded(availState, podState HealthState, evidences []rca.Evidence) string {
	reasons := make([]string, 0, 2)

	// Availability 部分不可用
	if availState == HealthStateDegraded {
		for _, e := range evidences {
			if !isAvailabilitySignal(e) {
				continue
			}
			avail := parseAvailability(e.Expected)
			if avail.valid && avail.desired > 0 && avail.ready < avail.desired {
				reasons = append(reasons, fmt.Sprintf("%s availability: %d/%d ready",
					strings.TrimSuffix(string(e.Type), "_availability"),
					avail.ready, avail.desired))
				break
			}
		}
	}

	// Pod 级异常
	if podState == HealthStateDegraded {
		abnormalTypes := make(map[string]int)
		for _, e := range evidences {
			if isPodAbnormalSignal(e) {
				abnormalTypes[string(e.Type)]++
			}
		}
		for t, count := range abnormalTypes {
			reasons = append(reasons, fmt.Sprintf("%d %s", count, t))
		}
	}

	if len(reasons) == 0 {
		return "service is degraded"
	}
	return strings.Join(reasons, "; ")
}

// reasonForHealthy 生成 Healthy 状态的 Reason。
func reasonForHealthy(evidences []rca.Evidence) string {
	availCount := 0
	for _, e := range evidences {
		if isAvailabilitySignal(e) {
			availCount++
		}
	}
	if availCount > 0 {
		return fmt.Sprintf("all %d workload availability signals healthy", availCount)
	}
	return fmt.Sprintf("%d fresh evidence signals, no anomalies detected", len(evidences))
}

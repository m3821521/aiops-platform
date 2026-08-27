// Package health 实现 Service Health Evaluation Engine。
//
// 核心原则：
//   - Health Evaluation 是纯业务计算层，不修改输入 Evidence。
//   - 只有 TrustStatus=fresh 的 Evidence 可以作为主要健康判断依据。
//   - no data != healthy, stale != healthy, error != healthy, empty != healthy。
//   - 单个 OOMKilled / CrashLoop 不能直接把整个 Service 判断为 Down，必须结合 workload availability。
//   - Down > Degraded > Healthy，Unknown 不能被 Healthy 覆盖。
package health

import (
	"time"

	"github.com/aiops/aiops-platform/internal/rca"
)

// HealthState 是 Service Health 状态。
type HealthState string

const (
	// HealthStateHealthy 所有有效 fresh evidence 均正常。
	HealthStateHealthy HealthState = "healthy"
	// HealthStateDegraded 存在部分异常但服务仍在运行。
	HealthStateDegraded HealthState = "degraded"
	// HealthStateDown 服务完全不可用（workload availability=0）。
	HealthStateDown HealthState = "down"
	// HealthStateUnknown 无法可靠判断健康状态（无 fresh evidence / 只有 stale/error/empty）。
	HealthStateUnknown HealthState = "unknown"
)

// HealthEvaluation 是 Health Evaluation Engine 的输出。
//
// 不复制 Evidence 内容，只保留 EvidenceIDs 作为引用。
// Evidence 仍然是唯一事实来源。
type HealthEvaluation struct {
	State       HealthState `json:"state"`
	Reason      string      `json:"reason"`
	EvidenceIDs []string    `json:"evidence_ids"`
	EvaluatedAt time.Time   `json:"evaluated_at"`
}

// HealthEvaluator 是 Health Evaluation Engine 的接口。
// 接收 []rca.Evidence，返回 HealthEvaluation。
// 纯函数：不修改输入，不产生副作用。
type HealthEvaluator interface {
	Evaluate(evidences []rca.Evidence) HealthEvaluation
}

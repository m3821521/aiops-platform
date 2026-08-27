package health

import (
	"strconv"
	"strings"

	"github.com/aiops/aiops-platform/internal/rca"
)

// availabilityInfo 是从 workload availability Evidence 中解析出的信息。
type availabilityInfo struct {
	ready   int
	desired int
	valid   bool
}

// parseAvailability 从 Evidence 的 Expected 字段解析 ready/desired。
//
// 支持的格式（来自 signals/kubernetes.go）：
//   - "2/3 replicas ready (2 available)"  (deployment)
//   - "2/3 replicas ready"                 (statefulset)
//   - "2/3 nodes ready"                    (daemonset)
//
// 返回 valid=false 表示无法解析，该 Evidence 不应作为 availability 判断依据。
func parseAvailability(expected string) availabilityInfo {
	if expected == "" {
		return availabilityInfo{}
	}
	// 取第一个空格前的部分，如 "2/3"
	parts := strings.SplitN(expected, " ", 2)
	if len(parts) == 0 {
		return availabilityInfo{}
	}
	ratio := parts[0]
	nums := strings.Split(ratio, "/")
	if len(nums) != 2 {
		return availabilityInfo{}
	}
	ready, err := strconv.Atoi(nums[0])
	if err != nil {
		return availabilityInfo{}
	}
	desired, err := strconv.Atoi(nums[1])
	if err != nil {
		return availabilityInfo{}
	}
	return availabilityInfo{ready: ready, desired: desired, valid: true}
}

// isFresh 判断 Evidence 是否为 fresh 状态。
// 只有 fresh evidence 可以作为主要健康判断依据。
func isFresh(e rca.Evidence) bool {
	return e.TrustStatus == "fresh"
}

// isAvailabilitySignal 判断 Evidence 是否为 workload availability 信号。
func isAvailabilitySignal(e rca.Evidence) bool {
	switch string(e.Type) {
	case "deployment_availability", "statefulset_availability", "daemonset_availability":
		return true
	}
	return false
}

// isPodAbnormalSignal 判断 Evidence 是否为 Pod 级异常信号。
// 这些信号会导致 Degraded，但不能单独导致 Down。
func isPodAbnormalSignal(e rca.Evidence) bool {
	switch string(e.Type) {
	case "crashloop", "oomkilled", "image_pull":
		return true
	case "pod_readiness":
		// NotReady 的 pod_readiness 是 corroborating level
		return e.Level == rca.EvidenceLevelCorroborating
	case "pod_restart":
		// 重启次数过多也是异常
		return e.Level == rca.EvidenceLevelCorroborating
	}
	return false
}

// evaluateAvailability 评估 workload availability 信号，返回候选 HealthState。
//
// 规则：
//   - ready == 0 => Down
//   - ready < desired => Degraded
//   - ready == desired => Healthy (该信号正常)
//   - 无法解析 => Unknown (不影响判断)
//
// 注意：只评估 fresh evidence。
func evaluateAvailability(evidences []rca.Evidence) (HealthState, []string) {
	var ids []string
	candidate := HealthStateUnknown

	for _, e := range evidences {
		if !isFresh(e) || !isAvailabilitySignal(e) {
			continue
		}
		ids = append(ids, e.ID)
		avail := parseAvailability(e.Expected)
		if !avail.valid {
			continue
		}
		if avail.desired > 0 && avail.ready == 0 {
			// 完全不可用 => Down
			if severityRank(HealthStateDown) > severityRank(candidate) {
				candidate = HealthStateDown
			}
		} else if avail.desired > 0 && avail.ready < avail.desired {
			// 部分不可用 => Degraded
			if severityRank(HealthStateDegraded) > severityRank(candidate) {
				candidate = HealthStateDegraded
			}
		} else if avail.desired > 0 && avail.ready == avail.desired {
			// 全部可用 => Healthy (该信号正常)
			if candidate == HealthStateUnknown {
				candidate = HealthStateHealthy
			}
		}
	}

	return candidate, ids
}

// evaluatePodAbnormal 评估 Pod 级异常信号，返回候选 HealthState。
//
// 规则：
//   - 存在任何 Pod 级异常 => Degraded
//   - 不存在 Pod 级异常 => Unknown (不影响判断)
//
// 注意：Pod 级异常不能单独导致 Down，必须结合 workload availability。
// 只评估 fresh evidence。
func evaluatePodAbnormal(evidences []rca.Evidence) (HealthState, []string) {
	var ids []string
	hasAbnormal := false

	for _, e := range evidences {
		if !isFresh(e) || !isPodAbnormalSignal(e) {
			continue
		}
		ids = append(ids, e.ID)
		hasAbnormal = true
	}

	if hasAbnormal {
		return HealthStateDegraded, ids
	}
	return HealthStateUnknown, ids
}

// severityRank 返回 HealthState 的严重度排名，用于优先级比较。
// Down > Degraded > Healthy > Unknown
func severityRank(s HealthState) int {
	switch s {
	case HealthStateDown:
		return 3
	case HealthStateDegraded:
		return 2
	case HealthStateHealthy:
		return 1
	default:
		return 0
	}
}

// mergeState 合并两个候选状态，返回严重度更高的。
// Unknown 不能被 Healthy 覆盖（如果 a=Unknown, b=Healthy => Unknown）。
// 但如果 a=Healthy, b=Unknown => Healthy（因为已经有 Healthy 证据）。
func mergeState(a, b HealthState) HealthState {
	if a == HealthStateUnknown {
		return b
	}
	if b == HealthStateUnknown {
		return a
	}
	if severityRank(a) >= severityRank(b) {
		return a
	}
	return b
}

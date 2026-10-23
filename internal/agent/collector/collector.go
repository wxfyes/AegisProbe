package collector

import (
	"tianque_probe/internal/common"
)

// SystemCollector 接口定义指标采集能力
type SystemCollector interface {
	Collect() (*common.MetricsData, error)
}

// NewCollector 工厂函数根据平台返回最优采集器
func NewCollector() SystemCollector {
	return newPlatformCollector()
}

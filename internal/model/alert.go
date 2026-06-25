package model

import "time"

// AlertRule defines a monitoring alert rule
type AlertRule struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`      // 规则名称
	Metric    string    `json:"metric"`    // cpu_percent, mem_percent, disk_percent, load_1m
	Threshold float64   `json:"threshold"` // 阈值
	Duration  int       `json:"duration"`  // 持续秒数（连续超过阈值才触发）
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AlertEvent represents a triggered alert
type AlertEvent struct {
	Rule      AlertRule `json:"rule"`
	Value     float64   `json:"value"`     // 当前值
	Threshold float64   `json:"threshold"` // 阈值
	Timestamp string    `json:"timestamp"`
	Message   string    `json:"message"`
}

// AlertState tracks the state of an alert rule evaluation
type AlertState struct {
	FirstAbove time.Time // 首次超过阈值的时间
	Triggered  bool      // 是否已触发通知
}

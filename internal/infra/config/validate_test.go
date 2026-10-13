package config

import "testing"

// 日志配置校验：非法等级/格式必须被拒绝（避免静默回退导致等级失效）。

func TestValidLogLevel(t *testing.T) {
	valid := []string{"debug", "info", "warn", "warning", "error", ""}
	for _, v := range valid {
		if !validLogLevel(v) {
			t.Errorf("validLogLevel(%q) 应为 true", v)
		}
	}
	invalid := []string{"verbose", "INFO2", "panic", "all", " "}
	for _, v := range invalid {
		if validLogLevel(v) {
			t.Errorf("validLogLevel(%q) 应为 false", v)
		}
	}
}

func TestValidLogFormat(t *testing.T) {
	for _, v := range []string{"text", "json", ""} {
		if !validLogFormat(v) {
			t.Errorf("validLogFormat(%q) 应为 true", v)
		}
	}
	for _, v := range []string{"yaml", "xml", " "} {
		if validLogFormat(v) {
			t.Errorf("validLogFormat(%q) 应为 false", v)
		}
	}
}

func TestValidateRejectsBadLogConfig(t *testing.T) {
	cfg := &Config{}
	cfg.Logs.Level = "verbose" // 非法等级
	if err := validateLogConfig(cfg); err == nil {
		t.Error("非法日志等级应被拒绝")
	}
	cfg.Logs.Level = "info"
	cfg.Logs.Format = "yaml" // 非法格式
	if err := validateLogConfig(cfg); err == nil {
		t.Error("非法日志格式应被拒绝")
	}
	cfg.Logs.Format = "text"
	if err := validateLogConfig(cfg); err != nil {
		t.Errorf("合法日志配置应通过: %v", err)
	}
}

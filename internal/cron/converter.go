package cron

import (
	"fmt"
	"strings"
)

// ScheduleForm 是 UI 调度表单的结构化表示，Converter 转为 systemd OnCalendar 表达式。
// 预设频率 + 时间点，从源头规避 cron→OnCalendar 的 esoteric 映射难题（ADR-0004）。
//
// Frequency 取值：
//   - minutely：每 N 分钟（EveryN）
//   - hourly：每 N 小时（EveryN）
//   - daily：每天 Time（HH:MM）
//   - weekly：每周 Weekdays + Time
//   - monthly：每月 DayOfMonth + Time
type ScheduleForm struct {
	Frequency  string   `json:"frequency"`
	EveryN     int      `json:"every_n,omitempty"`      // minutely/hourly 的步长
	Time       string   `json:"time,omitempty"`         // "HH:MM"，daily/weekly/monthly 用
	Weekdays   []string `json:"weekdays,omitempty"`     // ["Mon","Wed"]，weekly 用
	DayOfMonth int      `json:"day_of_month,omitempty"` // 1-31，monthly 用
}

// BuildOnCalendar 把 UI 调度表单转为 systemd OnCalendar 表达式。
// 纯函数，无副作用，便于测试。语言参数用于校验（可选，空则跳过校验）。
func BuildOnCalendar(f ScheduleForm) (string, error) {
	switch f.Frequency {
	case "minutely":
		if f.EveryN < 1 {
			return "", fmt.Errorf("每 N 分钟要求 N ≥ 1")
		}
		return fmt.Sprintf("*:00/%d", f.EveryN), nil
	case "hourly":
		if f.EveryN < 1 {
			return "", fmt.Errorf("每 N 小时要求 N ≥ 1")
		}
		return fmt.Sprintf("*-*-* 0/%d:00:00", f.EveryN), nil
	case "daily":
		hh, mm, err := parseTimeHM(f.Time)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("*-*-* %02d:%02d:00", hh, mm), nil
	case "weekly":
		hh, mm, err := parseTimeHM(f.Time)
		if err != nil {
			return "", err
		}
		days, err := validateWeekdays(f.Weekdays)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s *-*-* %02d:%02d:00", strings.Join(days, ","), hh, mm), nil
	case "monthly":
		hh, mm, err := parseTimeHM(f.Time)
		if err != nil {
			return "", err
		}
		if f.DayOfMonth < 1 || f.DayOfMonth > 31 {
			return "", fmt.Errorf("每月固定日必须在 1-31 之间")
		}
		return fmt.Sprintf("*-*-%02d %02d:%02d:00", f.DayOfMonth, hh, mm), nil
	default:
		return "", fmt.Errorf("不支持的频率: %s", f.Frequency)
	}
}

// DescribeSchedule 把调度表单转成人类可读的中文描述（UI 回显用）。
func DescribeSchedule(f ScheduleForm) string {
	switch f.Frequency {
	case "minutely":
		return fmt.Sprintf("每 %d 分钟执行", f.EveryN)
	case "hourly":
		return fmt.Sprintf("每 %d 小时执行", f.EveryN)
	case "daily":
		return fmt.Sprintf("每天 %s 执行", f.Time)
	case "weekly":
		return fmt.Sprintf("每周 %s 的 %s 执行", strings.Join(f.Weekdays, "、"), f.Time)
	case "monthly":
		return fmt.Sprintf("每月 %d 号 %s 执行", f.DayOfMonth, f.Time)
	default:
		return "未知调度"
	}
}

// parseTimeHM 解析 "HH:MM" 时间串，返回时/分。
func parseTimeHM(t string) (int, int, error) {
	parts := strings.Split(t, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("时间格式应为 HH:MM，收到 %q", t)
	}
	var hh, mm int
	if _, err := fmt.Sscanf(parts[0], "%d", &hh); err != nil || hh < 0 || hh > 23 {
		return 0, 0, fmt.Errorf("小时无效: %q", parts[0])
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &mm); err != nil || mm < 0 || mm > 59 {
		return 0, 0, fmt.Errorf("分钟无效: %q", parts[1])
	}
	return hh, mm, nil
}

// validateWeekdays 校验并规范星期列表（"Mon".."Sun" 三字母缩写，乱序去重）。
func validateWeekdays(days []string) ([]string, error) {
	if len(days) == 0 {
		return nil, fmt.Errorf("每周调度至少需要一个星期")
	}
	valid := map[string]bool{
		"Mon": true, "Tue": true, "Wed": true, "Thu": true,
		"Fri": true, "Sat": true, "Sun": true,
	}
	seen := map[string]bool{}
	var out []string
	for _, d := range days {
		d = strings.TrimSpace(d)
		if !valid[d] {
			return nil, fmt.Errorf("无效的星期 %q（应为 Mon..Sun）", d)
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out, nil
}

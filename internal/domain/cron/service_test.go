package cron

import (
	"testing"
)

// reconcile 是失败翻转状态机（首次不通知 / 翻转通知 / 持续失败不重复 / 恢复后
// 再失败又通知）。通知由调用方（sweep）根据返回的翻转列表发送。

func TestReconcileTaskFailureFlips(t *testing.T) {
	svc := &Service{lastSeen: make(map[string]string)}

	// 首次扫描：只建基线，不通知（存量失败不刷屏）
	got := svc.reconcile(map[string]string{"job-a": "success", "job-b": "exit-code", "job-c": "success"})
	if len(got) != 0 {
		t.Fatalf("first scan must not notify, got %v", got)
	}

	// 第二次：job-c 从 success 翻转为 exit-code → 通知；job-b 持续失败不重复
	got = svc.reconcile(map[string]string{"job-a": "success", "job-b": "exit-code", "job-c": "exit-code"})
	if len(got) != 1 || got[0] != "job-c" {
		t.Fatalf("expected only job-c to flip, got %v", got)
	}

	// 第三次：job-c 仍失败（持续），不重复
	got = svc.reconcile(map[string]string{"job-a": "success", "job-b": "exit-code", "job-c": "exit-code"})
	if len(got) != 0 {
		t.Fatalf("persistent failure must not re-notify, got %v", got)
	}

	// job-c 恢复为 success（更新基线，不通知）
	got = svc.reconcile(map[string]string{"job-a": "success", "job-b": "exit-code", "job-c": "success"})
	if len(got) != 0 {
		t.Fatalf("recovery must not notify, got %v", got)
	}

	// job-c 再次失败（signal）：翻转，通知
	got = svc.reconcile(map[string]string{"job-a": "success", "job-b": "exit-code", "job-c": "signal"})
	if len(got) != 1 || got[0] != "job-c" {
		t.Fatalf("expected job-c to flip again, got %v", got)
	}
}

func TestTaskFailedResult(t *testing.T) {
	cases := []struct {
		result string
		want   bool
	}{
		{"success", false},
		{"", false},
		{"exit-code", true},
		{"signal", true},
		{"timeout", true},
		{"start-limit-hit", true},
	}
	for _, tc := range cases {
		if got := taskFailedResult(tc.result); got != tc.want {
			t.Errorf("taskFailedResult(%q) = %v, want %v", tc.result, got, tc.want)
		}
	}
}

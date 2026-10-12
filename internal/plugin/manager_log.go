package plugin

import (
	"fmt"
	"log"
	"os"
	"time"
)

// ManagerLogPath 插件管理子系统全局日志（面板侧管理动作：扫描/安装/启停/
// 重启/健康/异常/事件/服务）。与单一插件日志互补：
//   - 本文件：面板对插件全体的管理操作日志（后端 logManagerf 写入）
//   - /var/log/easyserver/plugins/<id>.log：某个插件进程自身经 host.Log 的输出
//
// 面板经 GET /api/plugins/system-logs 读取（tail 查看）。
const ManagerLogPath = "/var/log/easyserver/plugin-manager.log"

// logManagerf 写插件管理全局日志：追加到专用文件 + 面板标准日志（journald）。
func logManagerf(format string, args ...any) {
	line := fmt.Sprintf("%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
	if f, err := os.OpenFile(ManagerLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640); err == nil {
		_, _ = f.WriteString(line)
		_ = f.Close()
	}
	log.Printf("plugin: "+format, args...)
}

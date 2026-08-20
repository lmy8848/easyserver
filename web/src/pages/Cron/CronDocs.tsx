import { Drawer, Collapse, theme } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

interface CronDocsProps {
  visible: boolean;
  onClose: () => void;
}

// 内置使用手册（前端写死，无需后端存储）。
const DOCS = [
  {
    key: 'schedule',
    title: '调度表达式',
    content: `## 调度表达式

任务通过「自定义表达式」直接填写调度语法，也可用「预设频率」自动生成。表达式采用 systemd 的 OnCalendar 格式，由**星期、日期、时间**三部分构成，均可省略：

| 部分 | 写法 | 示例 |
|------|------|------|
| 星期 | 周一至周日缩写 | Mon / Mon..Fri |
| 日期 | 年-月-日，可简写为月-日或日 | 2026-08-08 / 08-08 / 08 |
| 时间 | 时:分:秒，可省略秒 | 03:00:00 / 03:00 |

### 通配与步长

- \`*\` 匹配任意值
- \`..\` 范围，如 \`Mon..Fri\`
- \`,\` 列举多个值，如 \`Mon,Wed,Fri\`
- \`/\` 步长，如 \`*:00/5\` 每隔 5 分钟

### 常用示例

| 表达式 | 含义 |
|--------|------|
| \`*:*:0/5\` | 每 5 秒 |
| \`*:00/5\` | 每 5 分钟 |
| \`*:00/15\` | 每 15 分钟 |
| \`*-*-* 0/3:00:00\` | 每 3 小时 |
| \`*-*-* 03:00:00\` | 每天 03:00 |
| \`*-*-01/3 03:00:00\` | 每 3 天 03:00（从每月 1 号起） |
| \`Mon..Fri *-*-* 09:00:00\` | 工作日 09:00 |
| \`Mon,Wed,Fri *-*-* 09:00:00\` | 周一、三、五 09:00 |
| \`*-*-01 03:00:00\` | 每月 1 号 03:00 |

提交前会用 \`systemd-analyze\` 校验表达式合法性，并显示下次执行时间。

> **步长边界**：每 N 秒/分钟/小时按「每 N 个时间单位」触发，但以分钟/天为界重置。例如每 7 秒 = 每分钟内 0、7、14…56 秒，到 56 秒后跳到下一分钟的 0 秒；每 3 天 = 每月 1、4、7…号。N 不能整除 60（秒/分钟）或 30（天）时，边界处间隔会略短。`,
  },
  {
    key: 'preset',
    title: '预设频率',
    content: `## 预设频率

不熟悉表达式时，可直接选用预设频率，系统自动生成表达式：

| 频率 | 说明 |
|------|------|
| 每 N 秒 / 分钟 / 小时 | 按固定间隔触发，如每 5 分钟、每 3 小时 |
| 每 N 天 | 每隔 N 天 + 固定时间触发（从每月 1 号起） |
| 每天 | 每天固定时间触发 |
| 每周 | 每周固定几天 + 时间触发 |
| 每月 | 每月固定日 + 时间触发 |

「每 N」类频率以分钟/天为界重置（如每 7 秒按每分钟循环），边界处可能略不精确，见「调度表达式」。

预设频率最终会转成表达式保存，可在任务列表中查看。`,
  },
  {
    key: 'persistent',
    title: '持久化执行',
    content: `## 持久化执行

开启后，若系统在预定触发时间处于关机或休眠状态，错过的执行计划将在下次开机时自动补齐执行。
适合日志轮转、数据备份等不宜漏跑的周期性任务。默认关闭（严格按计划时间执行）。

任务失败后会自动重试至最大次数，运行日志可在任务详情页查看。`,
  },
  {
    key: 'shell',
    title: 'Shell 脚本技巧',
    content: `## Shell 脚本技巧

### 严谨模式与执行环境固化

~~~bash
#!/bin/bash
set -euo pipefail   # 出错即停、未定义变量报错、管道任一失败即失败
IFS=$'\\n\\t'          # 关闭默认空格分割，避免文件名含空格被拆
export PATH="/usr/local/bin:/usr/bin:/bin"
umask 022
~~~

### 防重入锁（flock，原子且自动释放）

~~~bash
exec 9>"$0.lock"
flock -n 9 || { echo "已有实例在运行"; exit 1; }
# 进程退出时锁自动释放，无需 trap 清理
~~~

### 退出码规范

~~~bash
exit 0   # 成功
exit 1   # 可重试的失败（建议任务重试）
exit 2   # 致命错误，不应重试
~~~
`,
  },
];

export default function CronDocs({ visible, onClose }: CronDocsProps) {
  const { token } = theme.useToken();

  return (
    <Drawer
      title={<span><QuestionCircleOutlined /> 使用手册</span>}
      open={visible}
      onClose={onClose}
      size={640}
      zIndex={1100}
    >
      <Collapse
        defaultActiveKey={DOCS.map(d => d.key)}
        ghost
        items={DOCS.map(doc => ({
          key: doc.key,
          label: <strong>{doc.title}</strong>,
          children: (
            <div style={{ fontSize: 14, lineHeight: 1.8, color: token.colorText }}>
              <Markdown
                remarkPlugins={[remarkGfm]}
                components={{
                  table: ({ children }) => (
                    <table
                      style={{
                        borderCollapse: 'collapse',
                        width: '100%',
                        marginBottom: 16,
                        border: `1px solid ${token.colorBorderSecondary}`,
                      }}
                    >
                      {children}
                    </table>
                  ),
                  th: ({ children }) => (
                    <th
                      style={{
                        border: `1px solid ${token.colorBorderSecondary}`,
                        padding: '8px 12px',
                        background: token.colorFillAlter,
                        color: token.colorText,
                        fontWeight: 600,
                        textAlign: 'left',
                      }}
                    >
                      {children}
                    </th>
                  ),
                  td: ({ children }) => (
                    <td
                      style={{
                        border: `1px solid ${token.colorBorderSecondary}`,
                        padding: '8px 12px',
                        color: token.colorText,
                      }}
                    >
                      {children}
                    </td>
                  ),
                  blockquote: ({ children }) => (
                    <blockquote
                      style={{
                        borderLeft: `4px solid ${token.colorPrimary}`,
                        margin: '16px 0',
                        padding: '8px 16px',
                        background: token.colorFillAlter,
                        color: token.colorTextSecondary,
                        borderRadius: token.borderRadiusSM,
                      }}
                    >
                      {children}
                    </blockquote>
                  ),
                  code: ({ children, className }) => {
                    const isInline = !className;
                    return isInline ? (
                      <code
                        style={{
                          background: token.colorFillSecondary,
                          color: token.colorText,
                          padding: '2px 6px',
                          borderRadius: token.borderRadiusSM,
                          fontSize: 13,
                          fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
                        }}
                      >
                        {children}
                      </code>
                    ) : (
                      <code
                        style={{
                          display: 'block',
                          background: token.colorFillAlter,
                          color: token.colorText,
                          padding: 16,
                          borderRadius: token.borderRadiusLG,
                          fontSize: 13,
                          fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
                          overflowX: 'auto',
                        }}
                      >
                        {children}
                      </code>
                    );
                  },
                  pre: ({ children }) => (
                    <pre
                      style={{
                        background: token.colorFillAlter,
                        border: `1px solid ${token.colorBorderSecondary}`,
                        padding: 0,
                        borderRadius: token.borderRadiusLG,
                        overflow: 'auto',
                        marginBottom: 16,
                      }}
                    >
                      {children}
                    </pre>
                  ),
                }}
              >
                {doc.content}
              </Markdown>
            </div>
          ),
        }))}
      />
    </Drawer>
  );
}
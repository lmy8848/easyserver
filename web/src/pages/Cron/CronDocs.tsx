import { Drawer, Collapse } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { MARKDOWN_STYLES } from './types';

interface CronDocsProps {
  visible: boolean;
  onClose: () => void;
}

// 内置使用手册（前端写死，无需后端存储）。
const DOCS = [
  {
    key: 'schedule',
    title: '调度频率',
    content: `## 调度频率

定时任务通过预设频率设定执行计划，无需手工编写调度表达式：

| 频率 | 说明 |
|------|------|
| 每 N 分钟 | 按分钟步长触发，如每 5 分钟 |
| 每 N 小时 | 按小时步长触发，如每 3 小时 |
| 每天 | 每天固定时间触发 |
| 每周 | 每周固定几天 + 时间触发 |
| 每月 | 每月固定日 + 时间触发 |

选择频率后，系统会自动生成对应的执行计划，并在任务列表中显示下次执行时间。`,
  },
  {
    key: 'persistent',
    title: '持久化执行',
    content: `## 持久化执行

开启后，若系统在预定触发时间处于关机或休眠状态，错过的执行计划将在下次开机时自动补齐执行。
适合日志轮转、数据备份等不宜漏跑的周期性任务。默认关闭（严格按计划时间执行）。`,
  },
  {
    key: 'retry',
    title: '重试与超时',
    content: `## 重试与超时

- 任务失败后会自动重试，达到最大重试次数后停止。
- 超时时间用于限制单次执行的时长，卡住的任务不会无限运行。
- 运行时的日志会自动记录，可在任务详情页查看。`,
  },
  {
    key: 'shell',
    title: 'Shell 脚本技巧',
    content: `## Shell 脚本常用技巧

~~~bash
#!/bin/bash
set -e    # 遇到错误立即退出
set -o pipefail  # 管道中任何命令失败都算失败
~~~

### 防止重复执行
~~~bash
LOCK_FILE="/tmp/myscript.lock"
if [ -f "$LOCK_FILE" ]; then
    echo "脚本正在运行，退出"
    exit 1
fi
trap "rm -f $LOCK_FILE" EXIT
touch "$LOCK_FILE"
~~~

### 超时控制
~~~bash
timeout 300 long_running_command  # 5 分钟超时
~~~`,
  },
];

export default function CronDocs({ visible, onClose }: CronDocsProps) {
  return (
    <Drawer
      title={<span><QuestionCircleOutlined /> 使用手册</span>}
      open={visible}
      onClose={onClose}
      size={600}
    >
      <Collapse
        defaultActiveKey={DOCS.map(d => d.key)}
        ghost
        items={DOCS.map(doc => ({
          key: doc.key,
          label: <strong>{doc.title}</strong>,
          children: (
            <div style={{ fontSize: 14, lineHeight: 1.8 }}>
              <Markdown
                remarkPlugins={[remarkGfm]}
                components={{
                  table: ({children}) => <table style={MARKDOWN_STYLES.table}>{children}</table>,
                  th: ({children}) => <th style={MARKDOWN_STYLES.th}>{children}</th>,
                  td: ({children}) => <td style={MARKDOWN_STYLES.td}>{children}</td>,
                  code: ({children, className}) => {
                    const isInline = !className;
                    return isInline
                      ? <code style={MARKDOWN_STYLES.code}>{children}</code>
                      : <code style={{...MARKDOWN_STYLES.code, display: 'block', padding: 16}}>{children}</code>;
                  },
                  pre: ({children}) => <pre style={MARKDOWN_STYLES.pre}>{children}</pre>,
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
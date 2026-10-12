# 前端主题规范（Frontend Theming）

面板支持亮/暗双主题（`auto` / `light` / `dark`，见 `web/src/store/useThemeStore.ts`）。
暗色模式由 antd `ConfigProvider`（`darkAlgorithm`）与 `Layout.css` 里的 CSS 变量共同驱动。
**写界面时禁止用硬编码颜色破坏任一主题的对比度。**

## 主题驱动机制

- **antd 组件**：`App.tsx` 按 `isDark` 切换 `theme.darkAlgorithm` / `theme.defaultAlgorithm`，
  antd 组件自动跟随主题。
- **自定义元素**（`pre` / `div` 容器 / 文字）：用 `Layout.css` 的 CSS 变量
  （`:root` 亮色 + `[data-theme='dark']` 暗色两套），或 antd token（`theme.useToken()`）。

## 可用的语义 CSS 变量（`web/src/components/Layout.css`）

| 变量 | 用途 | 亮色 | 暗色 |
|------|------|------|------|
| `--bg-primary` | 页面主背景 | `#f0f2f5` | `#141414` |
| `--bg-secondary` | 内容/代码块次级背景 | `#ffffff` | `#1f1f1f` |
| `--bg-card` | 卡片背景 | `#ffffff` | `#1f1f1f` |
| `--border` | 边框 | `#e5e7eb` | `#303030` |
| `--text-primary` | 主文字 | `#1e293b` | `#f1f5f9` |
| `--text-secondary` | 次级文字 | `#64748b` | `#94a3b8` |
| `--text-muted` | 弱化文字 | `#94a3b8` | `#64748b` |
| `--text-disabled` | 禁用文字 | `#bfbfbf` | `#5c5c5c` |
| `--bg-warning` / `--text-warning` | 告警横幅 | `#fff7e6` / `#fa8c16` | 暗色琥珀 |
| `--success` / `--warning` / `--danger` | 语义色 | — | — |
| `--accent` / `--accent-glow` | 品牌强调 | — | — |

## 硬性规则

1. **背景 / 文字 / 边框**一律用 CSS 变量或 antd token，禁止内联硬编码 hex/rgba。
   例：`background: 'var(--bg-secondary)'`、`color: 'var(--text-muted)'`。
   需要回退值时写 `var(--x, #fallback)`（fallback 用亮色值）。
2. 拿 antd token 用 `theme.useToken()`（参考 `components/LogViewer/LogViewer.tsx`、
   `pages/Cron/CronLogs.tsx`）：`token.colorText`、`token.colorTextTertiary`、
   `token.colorBgContainer`、`token.colorFillAlter` 等。
3. **例外（允许硬编码，不主题化）**：
   - 终端式深底浅字（如插件日志 `#111`/`#9cdcfe`、编辑器 `#1e1e1e`/`#e0e0e0`）——两主题通用；
   - ECharts 图表系列色（`itemStyle.color` 等数据可视化色）；
   - 语义操作色（antd 预置的 `red`/`green`/`amber` 等）。

## 检查

新页面改动后跑 `pnpm tsc --noEmit` + `pnpm lint`；主题类改动用浏览器在亮/暗两主题下各过一眼，
重点看日志框、代码块、弱化文字、告警横幅。

历史债：早期代码曾大量内联硬编码浅背景/深灰字导致暗色不可见，已在一次全库 pass 中改为
CSS 变量（见 `25a9575`）。新增代码不要再引入。

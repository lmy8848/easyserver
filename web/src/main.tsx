import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { loadPluginStyles } from './utils/pluginStyles'

// 注入启用插件的样式贡献（品牌定制/换肤，面板自身样式之后生效）
void loadPluginStyles()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)

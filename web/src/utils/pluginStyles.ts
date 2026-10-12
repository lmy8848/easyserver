import { pluginsApi } from '../services/plugins';

// 插件样式注入：加载启用插件的 styles 贡献（<link> 注入 <head>）。
// 面板自身样式之后注入，使插件样式可覆盖默认外观（品牌定制/换肤）；
// 随插件启停增删对应 <link>（重新调用本函数即可同步）。
const loaded = new Map<string, HTMLLinkElement>();

export async function loadPluginStyles(): Promise<void> {
  try {
    const res = await pluginsApi.getStyles();
    const items = res.data.data?.items || [];
    const current = new Set(items.map((i) => i.url));

    // 移除已失效（插件禁用/移除）的样式
    for (const [url, link] of loaded) {
      if (!current.has(url)) {
        link.remove();
        loaded.delete(url);
      }
    }
    // 注入新增的样式
    for (const item of items) {
      if (loaded.has(item.url)) continue;
      const link = document.createElement('link');
      link.rel = 'stylesheet';
      link.href = item.url;
      document.head.appendChild(link);
      loaded.set(item.url, link);
    }
  } catch {
    // 样式加载失败不阻塞面板
  }
}

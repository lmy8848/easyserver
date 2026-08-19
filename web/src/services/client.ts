import axios from 'axios';

export const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
});

// 登录态走 HttpOnly Cookie（浏览自动携带），无需 JS 注入 token。
// 移动端等 header 场景由客户端自行附加，此处不处理。
api.interceptors.request.use(
  (config) => {
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// Response interceptor - handle errors
api.interceptors.response.use(
  (response) => {
    return response;
  },
  (error) => {
    if (error.response) {
      const { status, data } = error.response;

      if (status === 401) {
        // Cookie 失效/未登录 - don't redirect if already on login page
        if (!window.location.pathname.startsWith('/login')) {
          localStorage.removeItem('user');
          window.location.href = '/login';
        }
      }

      if (status === 429) {
        // Rate limit exceeded
        const msg = data?.message || '请求过于频繁，请稍后再试';
        import('antd').then(({ message }) => message.warning(msg));
      }

      // 后端错误响应统一为 { code, message, data }：把真实错误信息覆盖到
      // error.message 上，替代 axios 默认的 "Request failed with status code
      // NNN" 文案 —— 各 catch 块里的 error.message 直接显示后端消息，无需
      // 逐个调用点取 error.response.data.message。网络错误（无 response）
      // 不受影响，保持 "Network Error"/timeout 等原始信息。
      const serverMsg = data?.message;
      if (typeof serverMsg === 'string' && serverMsg) {
        error.message = serverMsg;
      }

      // blob 响应（下载/导出）出错时 body 仍是 JSON：读取后提取 message，
      // 否则这些调用点只能看到 "Request failed with status code NNN"。
      if (data instanceof Blob) {
        return data.text().then((text) => {
          try {
            const parsed = JSON.parse(text);
            if (parsed?.message) error.message = parsed.message;
          } catch { /* 非 JSON（如代理返回的 HTML 错误页）保持原样 */ }
          return Promise.reject(error);
        });
      }

      // Pass through original error so catch blocks can inspect error.response?.status
      return Promise.reject(error);
    }
    return Promise.reject(error);
  }
);

export default api;

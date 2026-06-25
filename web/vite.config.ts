import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const apiHost = env.VITE_API_HOST || 'localhost'
  const apiPort = env.VITE_API_PORT || '8080'
  const apiTarget = `http://${apiHost}:${apiPort}`
  const wsTarget = `ws://${apiHost}:${apiPort}`

  return {
    plugins: [react()],
    server: {
      host: '0.0.0.0',
      port: 5173,
      strictPort: true,
      proxy: {
        '/api': {
          target: apiTarget,
          changeOrigin: true,
        },
        '/ws': {
          target: wsTarget,
          ws: true,
          changeOrigin: true,
          // WebSocket 保活配置
          configure: (proxy) => {
            proxy.on('error', (err, _req, res) => {
              const timestamp = new Date().toISOString();
              console.error(`[${timestamp}] proxy error:`, err.message);
              // If res is a ServerResponse, send error status
              if ('statusCode' in res) {
                (res as import('http').ServerResponse).statusCode = 502;
                (res as import('http').ServerResponse).end('Bad Gateway');
              }
            });
            proxy.on('proxyReqWs', (_proxyReq, _req, socket) => {
              socket.on('error', (err) => {
                const timestamp = new Date().toISOString();
                console.error(`[${timestamp}] WebSocket socket error:`, err.message);
              });
            });
          },
        },
      },
    },
    build: {
      outDir: 'dist',
      sourcemap: false,
    },
  }
})

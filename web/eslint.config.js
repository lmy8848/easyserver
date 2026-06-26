import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores(['dist']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      globals: globals.browser,
    },
    rules: {
      // 降级为警告：any 类型是代码债，不是 CI 阻塞项
      '@typescript-eslint/no-explicit-any': 'warn',
      // 忽略以 _ 开头的未使用变量（常见于解构占位）；catch 块中允许 error/e 不使用
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_', caughtErrorsIgnorePattern: '^(error|e|_|_e)' }],
      // off：标准数据获取模式，useEffect + setState 是 React 社区广泛实践
      'react-hooks/set-state-in-effect': 'off',
    },
  },
])

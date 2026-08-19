import { create } from 'zustand';
import type { User } from '../types';
import { authApi } from '../services/auth';

// Validate user object shape from localStorage to prevent tampering
function isValidUser(obj: unknown): obj is User {
  if (!obj || typeof obj !== 'object') return false;
  const u = obj as Record<string, unknown>;
  return (
    typeof u['id'] === 'number' &&
    typeof u['username'] === 'string'
  );
}

interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;

  logout: () => void;
  loadUser: () => Promise<void>;
  updateUser: (user: User) => void;
}

// 登录态走 HttpOnly Cookie：JS 拿不到 token，登录态由 /auth/me 判定。
// user 缓存在 localStorage 仅用于首帧显示（must_change_pass 守卫），非安全边界。
function hydrateUser(): User | null {
  try {
    const raw = localStorage.getItem('user');
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    return isValidUser(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

export const useAuthStore = create<AuthState>((set) => ({
  user: hydrateUser(),
  isAuthenticated: false,
  // 启动即进入"判断中"：先请求 /auth/me 确认 cookie 登录态，期间不误跳登录页。
  isLoading: true,

  logout: () => {
    // 通知后端清 cookie + 黑名单（fire-and-forget，页面照常回登录页）
    authApi.logout().catch(() => {});
    localStorage.removeItem('user');
    set({
      user: null,
      isAuthenticated: false,
    });
  },

  loadUser: async () => {
    set({ isLoading: true });
    try {
      const res = await authApi.getProfile();
      const userData = res.data.data;
      if (!isValidUser(userData)) {
        throw new Error('Invalid user data from server');
      }
      set({
        user: userData,
        isAuthenticated: true,
        isLoading: false,
      });
    } catch (error: unknown) {
      // 仅 401 视为未登录；500 等保留（不误踢）
      const bizCode = (error as { response?: { data?: { code?: number } } })?.response?.data?.code;
      if (bizCode === 40100 || bizCode === 40101) {
        localStorage.removeItem('user');
        set({
          user: null,
          isAuthenticated: false,
          isLoading: false,
        });
      } else {
        set({
          isLoading: false,
        });
      }
    }
  },

  updateUser: (user: User) => {
    set({ user });
    localStorage.setItem('user', JSON.stringify(user));
  },
}));
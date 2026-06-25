import { create } from 'zustand';
import type { User } from '../types';
import { authApi } from '../services/api';

// Validate user object shape from localStorage to prevent tampering
function isValidUser(obj: unknown): obj is User {
  if (!obj || typeof obj !== 'object') return false;
  const u = obj as Record<string, unknown>;
  return (
    typeof u.id === 'number' &&
    typeof u.username === 'string' &&
    typeof u.role === 'string' &&
    ['admin', 'operator', 'viewer'].includes(u.role)
  );
}

interface AuthState {
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;

  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
  loadUser: () => Promise<void>;
  updateUser: (user: User) => void;
}

// SECURITY NOTE: Token is stored in localStorage for SPA compatibility.
// This is acceptable for single-admin panels but exposes token to XSS attacks.
// For multi-user production systems, consider migrating to httpOnly cookies.
export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  token: localStorage.getItem('token'),
  isAuthenticated: !!localStorage.getItem('token'),
  isLoading: false,

  login: async (username: string, password: string) => {
    set({ isLoading: true });
    try {
      const res = await authApi.login(username, password);
      const { token, user, must_change_pass } = res.data.data;

      if (!isValidUser(user)) {
        throw new Error('Invalid user data received');
      }

      localStorage.setItem('token', token);
      localStorage.setItem('user', JSON.stringify(user));

      // Merge must_change_pass into user object for client-side enforcement
      set({
        user: { ...user, must_change_pass },
        token,
        isAuthenticated: true,
        isLoading: false,
      });
    } catch (error) {
      set({ isLoading: false });
      throw error;
    }
  },

  logout: () => {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    localStorage.removeItem('must_change_pass'); // legacy cleanup
    set({
      user: null,
      token: null,
      isAuthenticated: false,
    });
  },

  loadUser: async () => {
    const token = localStorage.getItem('token');
    if (!token) {
      set({ isAuthenticated: false });
      return;
    }

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
      // Only clear token on 401 (unauthorized), not on 500 (server error)
      const errObj = error as Record<string, unknown> | undefined;
      if (errObj?.code === 40100 || errObj?.code === 40101) {
        localStorage.removeItem('token');
        localStorage.removeItem('user');
        set({
          user: null,
          token: null,
          isAuthenticated: false,
          isLoading: false,
        });
      } else {
        // Keep token, just set loading to false
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

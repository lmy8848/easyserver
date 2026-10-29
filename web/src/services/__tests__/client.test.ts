import { describe, it, expect } from 'vitest';
import { shouldRedirectToLogin } from '../client';

describe('shouldRedirectToLogin', () => {
  it('受保护页面在 401 时应回登录页', () => {
    expect(shouldRedirectToLogin('/')).toBe(true);
    expect(shouldRedirectToLogin('/files')).toBe(true);
    expect(shouldRedirectToLogin('/settings')).toBe(true);
  });

  it('登录页自身不再重定向（避免循环）', () => {
    expect(shouldRedirectToLogin('/login')).toBe(false);
    expect(shouldRedirectToLogin('/login?next=/files')).toBe(false);
  });

  it('外链分享页对匿名访客开放，不能被踢去登录页', () => {
    expect(shouldRedirectToLogin('/share/9d0c8e58')).toBe(false);
  });
});

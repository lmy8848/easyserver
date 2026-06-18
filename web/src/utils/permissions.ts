// 角色权限定义
export const ROLES = {
  ADMIN: 'admin',
  OPERATOR: 'operator',
  VIEWER: 'viewer',
} as const;

export type Role = typeof ROLES[keyof typeof ROLES];

// 权限定义
export const PERMISSIONS = {
  // 系统监控 - 所有角色
  MONITOR_VIEW: [ROLES.ADMIN, ROLES.OPERATOR, ROLES.VIEWER],

  // 服务管理
  SERVICE_VIEW: [ROLES.ADMIN, ROLES.OPERATOR, ROLES.VIEWER],
  SERVICE_MANAGE: [ROLES.ADMIN, ROLES.OPERATOR], // 启停重启

  // 终端访问
  TERMINAL_ACCESS: [ROLES.ADMIN, ROLES.OPERATOR],

  // 文件管理
  FILE_VIEW: [ROLES.ADMIN, ROLES.OPERATOR, ROLES.VIEWER],
  FILE_MANAGE: [ROLES.ADMIN, ROLES.OPERATOR], // 上传/编辑/删除

  // 用户管理
  USER_MANAGE: [ROLES.ADMIN],

  // 腾讯云
  CLOUD_VIEW: [ROLES.ADMIN, ROLES.OPERATOR, ROLES.VIEWER],
  CLOUD_MANAGE: [ROLES.ADMIN, ROLES.OPERATOR],

  // 操作日志
  AUDIT_VIEW: [ROLES.ADMIN],

  // 部署同步
  DEPLOY_MANAGE: [ROLES.ADMIN],

  // 网站管理
  WEBSITE_MANAGE: [ROLES.ADMIN],

  // 数据库管理
  DB_MANAGE: [ROLES.ADMIN],
} as const;

// 检查用户是否有指定权限
export function hasPermission(userRole: string | undefined, permission: readonly string[]): boolean {
  if (!userRole) return false;
  return permission.includes(userRole);
}

// 检查用户是否是管理员
export function isAdmin(userRole: string | undefined): boolean {
  return userRole === ROLES.ADMIN;
}

// 检查用户是否是操作员或管理员
export function isOperatorOrAbove(userRole: string | undefined): boolean {
  return userRole === ROLES.ADMIN || userRole === ROLES.OPERATOR;
}

// 角色显示名称
export const ROLE_LABELS: Record<string, string> = {
  [ROLES.ADMIN]: '管理员',
  [ROLES.OPERATOR]: '操作员',
  [ROLES.VIEWER]: '观察者',
};

// 角色颜色
export const ROLE_COLORS: Record<string, string> = {
  [ROLES.ADMIN]: 'red',
  [ROLES.OPERATOR]: 'blue',
  [ROLES.VIEWER]: 'green',
};

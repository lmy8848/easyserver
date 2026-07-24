import { lazy, Suspense, memo, useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ConfigProvider, Spin, theme, App as AntdApp, message, Modal, notification } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import Layout from './components/Layout';
import Login from './pages/Login';
import ChangePassword from './pages/ChangePassword';
import NotFound from './pages/NotFound';
import ErrorBoundary from './components/ErrorBoundary';
import { useAuthStore } from './store/useAuthStore';

// Lazy load pages for code splitting
const Dashboard = lazy(() => import('./pages/Dashboard'));
const Terminal = lazy(() => import('./pages/Terminal'));
const FileManager = lazy(() => import('./pages/FileManager'));
const Cloud = lazy(() => import('./pages/Cloud'));
const Deploy = lazy(() => import('./pages/Deploy'));
const AuditLog = lazy(() => import('./pages/AuditLog'));
const Settings = lazy(() => import('./pages/Settings'));
const Runtime = lazy(() => import('./pages/Runtime'));
const EnvConfig = lazy(() => import('./pages/EnvConfig'));
const Website = lazy(() => import('./pages/Website'));
const Database = lazy(() => import('./pages/Database'));
const Cron = lazy(() => import('./pages/Cron'));
const Script = lazy(() => import('./pages/Script'));
const Firewall = lazy(() => import('./pages/Firewall'));
const SecuritySettings = lazy(() => import('./pages/SecuritySettings'));
const Vulnerabilities = lazy(() => import('./pages/Security/Vulnerabilities'));
const LoginGuard = lazy(() => import('./pages/Security/LoginGuard'));
const FIM = lazy(() => import('./pages/Security/FIM'));
const SSH = lazy(() => import('./pages/SSH'));
const Container = lazy(() => import('./pages/Container'));
const ProcessGuardian = lazy(() => import('./pages/ProcessGuardian'));
const SystemMonitor = lazy(() => import('./pages/SystemMonitor'));
const Notifications = lazy(() => import('./pages/Notifications'));
const FileShares = lazy(() => import('./pages/FileShares'));
const ShareDownload = lazy(() => import('./pages/ShareDownload'));

const PageLoading = memo(function PageLoading() {
  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', minHeight: 200 }}>
      <Spin size="large" />
    </div>
  );
});

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, user } = useAuthStore();

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  // Enforce password change - check from server-side user object (not localStorage, which can be tampered)
  if (user?.must_change_pass && window.location.pathname !== '/change-password') {
    return <Navigate to="/change-password" replace />;
  }

  return <>{children}</>;
}

function StaticContextInjector() {
  const app = AntdApp.useApp();
  
  useEffect(() => {
    message.success = app.message.success;
    message.error = app.message.error;
    message.warning = app.message.warning;
    message.info = app.message.info;
    message.loading = app.message.loading;
    message.destroy = app.message.destroy;

    Modal.confirm = app.modal.confirm;
    Modal.info = app.modal.info;
    Modal.success = app.modal.success;
    Modal.error = app.modal.error;
    Modal.warning = app.modal.warning;

    notification.success = app.notification.success;
    notification.error = app.notification.error;
    notification.info = app.notification.info;
    notification.warning = app.notification.warning;
  }, [app]);
  
  return null;
}

function App() {
  return (
    <ConfigProvider 
      locale={zhCN} 
      theme={{ 
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: '#6366f1',
          borderRadius: 6,
          colorBgLayout: '#f0f2f5',
          colorBorderSecondary: '#e5e7eb',
        },
        components: {
          Table: {
            headerBg: '#f9fafb',
          }
        }
      }}
    >
      <AntdApp>
        <StaticContextInjector />
        <BrowserRouter>
        <ErrorBoundary>
          <Suspense fallback={<PageLoading />}>
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route path="/change-password" element={<ChangePassword />} />
              <Route path="/share/:token" element={<ShareDownload />} />
              <Route
                path="/"
                element={
                  <ProtectedRoute>
                    <Layout />
                  </ProtectedRoute>
                }
              >
                <Route index element={<Dashboard />} />
                <Route path="terminal" element={<Terminal />} />
                <Route path="files" element={<FileManager />} />
                <Route path="cloud" element={<Cloud />} />
                <Route path="deploy" element={<Deploy />} />
                <Route path="audit" element={<AuditLog />} />
                <Route path="settings" element={<Settings />} />
                <Route path="security" element={<SecuritySettings />} />
                <Route path="vulnerabilities" element={<Vulnerabilities />} />
                <Route path="login-guard" element={<LoginGuard />} />
                <Route path="fim" element={<FIM />} />
                <Route path="runtime" element={<Runtime />} />
                <Route path="env-config" element={<EnvConfig />} />
                <Route path="websites" element={<Website />} />
                <Route path="databases" element={<Database />} />
                <Route path="cron" element={<Cron />} />
                <Route path="scripts" element={<Script />} />
                <Route path="firewall" element={<Firewall />} />
                <Route path="ssh" element={<SSH />} />
                <Route path="containers" element={<Container />} />
                <Route path="processes" element={<ProcessGuardian />} />
                <Route path="system-monitor" element={<SystemMonitor />} />
                <Route path="notifications" element={<Notifications />} />
                <Route path="file-shares" element={<FileShares />} />
              </Route>
              <Route path="/404" element={<NotFound />} />
              <Route path="*" element={<NotFound />} />
            </Routes>
          </Suspense>
        </ErrorBoundary>
      </BrowserRouter>
      </AntdApp>
    </ConfigProvider>
  );
}

export default App;

import { lazy, Suspense, memo } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ConfigProvider, Spin } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import Layout from './components/Layout';
import Login from './pages/Login';
import ChangePassword from './pages/ChangePassword';
import NotFound from './pages/NotFound';
import ErrorBoundary from './components/ErrorBoundary';
import { useAuthStore } from './store/useAuthStore';

// Lazy load pages for code splitting
const Dashboard = lazy(() => import('./pages/Dashboard'));
const Services = lazy(() => import('./pages/Services'));
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
const SSH = lazy(() => import('./pages/SSH'));
const Container = lazy(() => import('./pages/Container'));
const ProcessGuardian = lazy(() => import('./pages/ProcessGuardian'));

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

function App() {
  return (
    <ConfigProvider locale={zhCN}>
      <BrowserRouter>
        <ErrorBoundary>
          <Suspense fallback={<PageLoading />}>
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route path="/change-password" element={<ChangePassword />} />
              <Route
                path="/"
                element={
                  <ProtectedRoute>
                    <Layout />
                  </ProtectedRoute>
                }
              >
                <Route index element={<Dashboard />} />
                <Route path="services" element={<Services />} />
                <Route path="terminal" element={<Terminal />} />
                <Route path="files" element={<FileManager />} />
                <Route path="cloud" element={<Cloud />} />
                <Route path="deploy" element={<Deploy />} />
                <Route path="audit" element={<AuditLog />} />
                <Route path="settings" element={<Settings />} />
                <Route path="security" element={<SecuritySettings />} />
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
              </Route>
              <Route path="/404" element={<NotFound />} />
              <Route path="*" element={<NotFound />} />
            </Routes>
          </Suspense>
        </ErrorBoundary>
      </BrowserRouter>
    </ConfigProvider>
  );
}

export default App;

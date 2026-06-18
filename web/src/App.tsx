import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import Layout from './components/Layout';
import Login from './pages/Login';
import ChangePassword from './pages/ChangePassword';
import Dashboard from './pages/Dashboard';
import Services from './pages/Services';
import Terminal from './pages/Terminal';
import FileManager from './pages/FileManager';
import Users from './pages/Users';
import Cloud from './pages/Cloud';
import Deploy from './pages/Deploy';
import AuditLog from './pages/AuditLog';
import Settings from './pages/Settings';
import Runtime from './pages/Runtime';
import EnvConfig from './pages/EnvConfig';
import Website from './pages/Website';
import Database from './pages/Database';
import { useAuthStore } from './store/useAuthStore';

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useAuthStore();

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  // Enforce password change for users with must_change_pass flag
  const mustChangePass = localStorage.getItem('must_change_pass') === 'true';
  if (mustChangePass && window.location.pathname !== '/change-password') {
    return <Navigate to="/change-password" replace />;
  }

  return <>{children}</>;
}

function App() {
  return (
    <ConfigProvider locale={zhCN}>
      <BrowserRouter>
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
            <Route path="users" element={<Users />} />
            <Route path="cloud" element={<Cloud />} />
            <Route path="deploy" element={<Deploy />} />
            <Route path="audit" element={<AuditLog />} />
            <Route path="settings" element={<Settings />} />
            <Route path="runtime" element={<Runtime />} />
            <Route path="env-config" element={<EnvConfig />} />
            <Route path="websites" element={<Website />} />
            <Route path="databases" element={<Database />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </ConfigProvider>
  );
}

export default App;

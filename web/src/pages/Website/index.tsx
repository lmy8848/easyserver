import { useState, useEffect, useRef } from 'react';
import { message } from 'antd';
import { webServerApi } from '../../services/api';
import type { WebServer } from '../../types';
import type { ConfigTestResult } from './types';
import type { ConfigEditorRef } from './ConfigEditor';
import WebServerList from './WebServerList';
import WebsiteList from './WebsiteList';
import ConfigEditor from './ConfigEditor';

export default function WebsitePage() {
  const [servers, setServers] = useState<WebServer[]>([]);
  const [loading, setLoading] = useState(false);
  const [selectedServer, setSelectedServer] = useState<WebServer | null>(null);

  // Operation loading state (for server-level actions)
  const [operating, setOperating] = useState<string>('');

  // Config test result (shared between WebsiteList header and ConfigEditor modal)
  const [configTestResult, setConfigTestResult] = useState<ConfigTestResult | null>(null);

  // ConfigEditor ref for calling showConfig/showServiceLogs from WebsiteList
  const configEditorRef = useRef<ConfigEditorRef>(null);

  // Ref to hold the latest selectedServer so the polling interval can read
  // current data without depending on the object identity. Without this,
  // setState({...prev}) creates a new reference every tick, the effect
  // re-runs, clears the interval, fires again immediately — an infinite
  // loop that burns through the API rate limit within seconds.
  const selectedServerRef = useRef<WebServer | null>(null);
  useEffect(() => { selectedServerRef.current = selectedServer; }, [selectedServer]);

  const fetchServers = async () => {
    setLoading(true);
    try {
      const res = await webServerApi.list();
      setServers(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch servers:', error);
    } finally {
      setLoading(false);
    }
  };

  // Fetch servers on mount
  useEffect(() => {
    fetchServers();
  }, []);

  // Auto-refresh process info when viewing server detail (every 10s).
  // Only the server ID is a dependency — the interval reads the latest
  // data through selectedServerRef so it is never re-created by setState.
  useEffect(() => {
    const server = selectedServerRef.current;
    if (!server || server.status === 'not_installed') return;

    const refresh = async () => {
      const current = selectedServerRef.current;
      if (!current) return;
      try {
        const [procRes, statusRes] = await Promise.all([
          webServerApi.getProcessInfo(current.id),
          webServerApi.status(current.id),
        ]);
        const proc = procRes.data.data;
        const status = statusRes.data.data;
        if (proc || status) {
          setSelectedServer(prev => prev ? { ...prev, ...proc, ...status } : prev);
          setServers(prev => prev.map(s =>
            s.id === current.id ? { ...s, ...proc, ...status } : s
          ));
        }
      } catch (error) {
        console.error('Failed to refresh server status:', error);
      }
    };

    refresh(); // immediate
    const timer = setInterval(refresh, 10000);
    return () => clearInterval(timer);
  }, [selectedServer?.id]);

  // Enter server detail
  const enterServer = async (server: WebServer) => {
    setSelectedServer(server);
  };

  const goBack = () => {
    setSelectedServer(null);
  };

  // Refresh a single server in the list (and selectedServer if viewing it)
  const refreshServer = async (serverId: number) => {
    try {
      const res = await webServerApi.get(serverId);
      const updated = res.data.data;
      setServers(prev => prev.map(s => s.id === serverId ? { ...s, ...updated } : s));
      setSelectedServer(prev => prev && prev.id === serverId ? { ...prev, ...updated } : prev);
    } catch (error) {
      console.error('Failed to refresh server:', serverId, error);
    }
  };

  // Server actions with loading and dynamic refresh
  const handleInstall = async (server: WebServer) => {
    setOperating(`install-${server.id}`);
    try {
      await webServerApi.install(server.id);
      message.success(`${server.display_name} 安装成功`);
      await refreshServer(server.id);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '安装失败'));
    } finally {
      setOperating('');
    }
  };

  const handleUninstall = async (server: WebServer) => {
    setOperating(`uninstall-${server.id}`);
    try {
      await webServerApi.uninstall(server.id);
      message.success(`${server.display_name} 已卸载`);
      await refreshServer(server.id);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '卸载失败'));
    } finally {
      setOperating('');
    }
  };

  const handleStart = async (server: WebServer) => {
    setOperating(`start-${server.id}`);
    try {
      await webServerApi.start(server.id);
      message.success(`${server.display_name} 已启动`);
      await refreshServer(server.id);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '启动失败'));
    } finally {
      setOperating('');
    }
  };

  const handleStop = async (server: WebServer) => {
    setOperating(`stop-${server.id}`);
    try {
      await webServerApi.stop(server.id);
      message.success(`${server.display_name} 已停止`);
      await refreshServer(server.id);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '停止失败'));
    } finally {
      setOperating('');
    }
  };

  const handleRestart = async (server: WebServer) => {
    setOperating(`restart-${server.id}`);
    try {
      await webServerApi.restart(server.id);
      message.success(`${server.display_name} 已重启`);
      await refreshServer(server.id);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '重启失败'));
    } finally {
      setOperating('');
    }
  };

  // Reload config
  const handleReload = async (server: WebServer) => {
    setOperating(`reload-${server.id}`);
    try {
      await webServerApi.reload(server.id);
      message.success('配置已重载');
      await refreshServer(server.id);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '重载失败'));
    } finally {
      setOperating('');
    }
  };

  // Test config
  const handleTestConfig = async (server: WebServer) => {
    setOperating(`test-${server.id}`);
    try {
      const res = await webServerApi.testConfig(server.id);
      setConfigTestResult(res.data.data || null);
    } catch (error: unknown) {
      setConfigTestResult({ valid: false, message: (error instanceof Error ? error.message : 'test failed') });
    } finally {
      setOperating('');
    }
  };

  // Level 1: Web Server cards
  if (!selectedServer) {
    return (
      <WebServerList
        servers={servers}
        loading={loading}
        operating={operating}
        onEnterServer={enterServer}
        onInstall={handleInstall}
        onStart={handleStart}
        onStop={handleStop}
        onRestart={handleRestart}
        onRefresh={fetchServers}
      />
    );
  }

  // Level 2: Website list for selected server
  return (
    <>
      <WebsiteList
        selectedServer={selectedServer}
        operating={operating}
        configTestResult={configTestResult}
        onGoBack={goBack}
        onStop={handleStop}
        onStart={handleStart}
        onRestart={handleRestart}
        onReload={handleReload}
        onInstall={handleInstall}
        onUninstall={handleUninstall}
        onTestConfig={handleTestConfig}
        onRefreshServer={refreshServer}
        onShowConfig={() => configEditorRef.current?.showConfig()}
        onShowServiceLogs={() => configEditorRef.current?.showServiceLogs()}
      />
      <ConfigEditor
        ref={configEditorRef}
        selectedServer={selectedServer}
        configTestResult={configTestResult}
        onTestConfig={handleTestConfig}
        onConfigTestResultChange={setConfigTestResult}
      />
    </>
  );
}

import { useEffect, useState } from 'react';
import { Button, Space } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { pluginsApi } from '../../services/plugins';

// 插件管理子系统全局日志：面板侧的管理动作（扫描/安装/启停/重启/健康/异常），
// 与单一插件日志（插件进程自身 host.Log 输出）互补。数据来自
// /var/log/easyserver/plugin-manager.log。
export default function PluginSystemLogs() {
  const [log, setLog] = useState('');
  const [loading, setLoading] = useState(false);

  const fetchLog = async () => {
    setLoading(true);
    try {
      const res = await pluginsApi.getSystemLogs(500);
      setLog(res.data.data?.log || '');
    } catch {
      setLog('获取日志失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void fetchLog(); }, []);

  return (
    <div>
      <Space style={{ marginBottom: 12 }}>
        <Button icon={<ReloadOutlined />} loading={loading} onClick={fetchLog}>刷新</Button>
        <span style={{ color: '#999', fontSize: 12 }}>面板侧插件管理动作日志（含插件启动/停止/重启/健康检查/异常）</span>
      </Space>
      <pre style={{ background: '#f6f6f6', padding: 12, borderRadius: 6, maxHeight: 480, overflow: 'auto', fontSize: 12 }}>
        {log || '暂无日志'}
      </pre>
    </div>
  );
}

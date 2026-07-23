import { useState, useEffect, useCallback } from 'react';
import { Card, Button, Table, Tag, Space, message, Popconfirm, Alert } from 'antd';
import { ReloadOutlined, ScanOutlined, CheckCircleOutlined, UndoOutlined } from '@ant-design/icons';
import api from '../../services/api';

interface FIMBaseline { path: string; hash: string; size: number; mtime: string; updated_at: string; }
interface FIMChange { path: string; change_type: string; old_hash: string; new_hash: string; detected_at: string; }

export default function FIM() {
  const [baseline, setBaseline] = useState<FIMBaseline[]>([]);
  const [changes, setChanges] = useState<FIMChange[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [b, c] = await Promise.all([
        api.get('/security/fim/baseline'),
        api.get('/security/fim/changes'),
      ]);
      setBaseline(b.data.data?.baseline || []);
      setChanges(c.data.data?.changes || []);
    } catch { message.error('加载失败'); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const scan = async () => {
    setBusy(true);
    try { await api.post('/security/fim/scan'); message.success('基线已建立'); load(); }
    catch (e: unknown) { message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '失败'); }
    finally { setBusy(false); }
  };

  const check = async () => {
    setBusy(true);
    try {
      const res = await api.post('/security/fim/check');
      const n = res.data.data?.count || 0;
      message.success(n > 0 ? `检测到 ${n} 处变更` : '未检测到变更');
      load();
    } catch (e: unknown) { message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '失败'); }
    finally { setBusy(false); }
  };

  const reset = async () => {
    setBusy(true);
    try { await api.post('/security/fim/reset'); message.success('基线已重置'); load(); }
    catch (e: unknown) { message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '失败'); }
    finally { setBusy(false); }
  };

  const changeColor = (t: string) => t === 'modified' ? 'orange' : t === 'deleted' ? 'red' : 'green';

  return (
    <Space direction="vertical" style={{ width: '100%' }} size="middle">
      <Card title="文件完整性监控" extra={<Button icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新</Button>}>
        <Alert message="监控关键文件（sshd_config / nginx.conf / config.yaml / authorized_keys）的 sha256 基线，检测修改/删除。" type="info" showIcon style={{ marginBottom: 16 }} />
        <Space>
          <Button type="primary" icon={<ScanOutlined />} onClick={scan} loading={busy}>建立基线</Button>
          <Button icon={<CheckCircleOutlined />} onClick={check} loading={busy}>检测变更</Button>
          <Popconfirm title="重置基线将清除当前基线并重新扫描，确定？" onConfirm={reset}>
            <Button icon={<UndoOutlined />} loading={busy}>重置基线</Button>
          </Popconfirm>
        </Space>
      </Card>

      <Card title={`变更记录 (${changes.length})`}>
        <Table size="small" dataSource={changes} rowKey={(r, i) => `${r.path}-${r.detected_at}-${i}`} loading={loading} pagination={{ pageSize: 20 }}
          locale={{ emptyText: '暂无变更' }}
          columns={[
            { title: '文件', dataIndex: 'path', key: 'path' },
            { title: '类型', dataIndex: 'change_type', key: 'change_type', width: 100, render: (t: string) => <Tag color={changeColor(t)}>{t}</Tag> },
            { title: '旧哈希', dataIndex: 'old_hash', key: 'old_hash', ellipsis: true, render: (h: string) => h ? h.substring(0, 16) + '...' : '-' },
            { title: '新哈希', dataIndex: 'new_hash', key: 'new_hash', ellipsis: true, render: (h: string) => h ? h.substring(0, 16) + '...' : '-' },
            { title: '检测时间', dataIndex: 'detected_at', key: 'detected_at', width: 180 },
          ]}
        />
      </Card>

      <Card title={`基线 (${baseline.length})`}>
        <Table size="small" dataSource={baseline} rowKey="path" loading={loading} pagination={false}
          locale={{ emptyText: '尚未建立基线，点击上方「建立基线」' }}
          columns={[
            { title: '文件', dataIndex: 'path', key: 'path' },
            { title: '哈希', dataIndex: 'hash', key: 'hash', ellipsis: true, render: (h: string) => h.substring(0, 16) + '...' },
            { title: '大小', dataIndex: 'size', key: 'size', width: 100 },
            { title: '修改时间', dataIndex: 'mtime', key: 'mtime', width: 180 },
            { title: '基线时间', dataIndex: 'updated_at', key: 'updated_at', width: 180 },
          ]}
        />
      </Card>
    </Space>
  );
}

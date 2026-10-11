import { useEffect, useState } from 'react';
import { Drawer, Descriptions, Tag, Tabs, Space, Button, Alert, Empty } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { pluginsApi, type PluginDetail } from '../../services/plugins';
import type { PluginRecord } from '../../types';

// 插件详情：基本信息 + 日志查看 + 贡献点 + manifest（插件功能全覆盖展示）
export default function PluginDetailDrawer({ plugin, open, onClose }: { plugin: PluginRecord; open: boolean; onClose: () => void }) {
  const [detail, setDetail] = useState<PluginDetail | null>(null);
  const [log, setLog] = useState('');
  const [logLoading, setLogLoading] = useState(false);

  useEffect(() => {
    if (!open || !plugin) return;
    setDetail(null);
    setLog('');
    pluginsApi.getDetail(plugin.name).then((res) => setDetail(res.data.data)).catch(() => {});
  }, [open, plugin]);

  const fetchLog = (lines = 500) => {
    if (!plugin) return;
    setLogLoading(true);
    pluginsApi.getLogs(plugin.name, lines).then((res) => {
      setLog(res.data.data?.log || '（暂无日志）');
    }).catch(() => setLog('（日志读取失败）')).finally(() => setLogLoading(false));
  };

  useEffect(() => {
    if (open && plugin) fetchLog(500);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, plugin]);

  const c = detail?.contributions;
  const perms = detail?.manifest?.permissions || plugin.permissions_approved || [];
  const record = detail?.record || plugin;

  const overview = (
    <Descriptions column={1} bordered size="small">
      <Descriptions.Item label="名称">{detail?.manifest?.name || plugin.name}</Descriptions.Item>
      <Descriptions.Item label="版本">{detail?.manifest?.version || plugin.version}</Descriptions.Item>
      <Descriptions.Item label="描述">{detail?.manifest?.description || '-'}</Descriptions.Item>
      <Descriptions.Item label="状态">
        <Tag color={record.status === 'running' ? 'green' : record.status === 'error' || record.status === 'degraded' ? 'red' : 'default'}>{record.status}</Tag>
        {record.enabled ? <Tag color="blue">已启用</Tag> : <Tag>未启用</Tag>}
      </Descriptions.Item>
      <Descriptions.Item label="PID">{record.pid || '-'}</Descriptions.Item>
      <Descriptions.Item label="重启次数">{record.restart_count}</Descriptions.Item>
      <Descriptions.Item label="最后错误">{record.last_error || '-'}</Descriptions.Item>
      <Descriptions.Item label="权限">
        <Space size={4} wrap>
          {perms.length ? perms.map((p) => <Tag key={p} color="purple" style={{ margin: 0 }}>{p}</Tag>) : '-'}
        </Space>
      </Descriptions.Item>
      {detail?.manifest?.runtime && (
        <Descriptions.Item label="运行时">
          exec: {detail.manifest.runtime.exec} · socket: {detail.manifest.runtime.socket}
        </Descriptions.Item>
      )}
    </Descriptions>
  );

  const logsTab = (
    <div>
      <Space style={{ marginBottom: 8 }}>
        <Button size="small" icon={<ReloadOutlined />} loading={logLoading} onClick={() => fetchLog(500)}>刷新</Button>
        <Button size="small" onClick={() => fetchLog(2000)}>更多</Button>
      </Space>
      <pre style={{ background: '#111', color: '#9cdcfe', padding: 12, borderRadius: 6, maxHeight: 480, overflow: 'auto', fontSize: 12 }}>
        {log}
      </pre>
      <Alert type="info" showIcon message="日志来自 /var/log/easyserver/plugins/（插件经 Host API log.write 写入）。" />
    </div>
  );

  const contributions = (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {c?.menus?.length ? (
        <div>
          <b>菜单</b>
          <Space size={4} wrap>{c.menus.map((m) => <Tag key={m.path} style={{ margin: 0 }}>{m.title} ({m.path})</Tag>)}</Space>
        </div>
      ) : null}
      {c?.pages?.length ? (
        <div>
          <b>页面</b>
          <Space size={4} wrap>{c.pages.map((p) => <Tag key={p.path} style={{ margin: 0 }}>{p.path}</Tag>)}</Space>
        </div>
      ) : null}
      {c?.routes?.length ? (
        <div>
          <b>路由</b>
          <Space size={4} wrap>{c.routes.map((r) => <Tag key={r.prefix} style={{ margin: 0 }}>{r.prefix}</Tag>)}</Space>
        </div>
      ) : null}
      {c?.widgets?.length ? (
        <div>
          <b>仪表盘小部件</b>
          <Space size={4} wrap>{c.widgets.map((w) => <Tag key={w.id} color="cyan" style={{ margin: 0 }}>{w.title} ({w.slot})</Tag>)}</Space>
        </div>
      ) : null}
      {c?.services?.length ? (
        <div>
          <b>后台服务</b>
          <Space size={4} wrap>{c.services.map((s) => <Tag key={s.id} color="orange" style={{ margin: 0 }}>{s.schedule} → {s.command}</Tag>)}</Space>
        </div>
      ) : null}
      {c?.eventsubs?.length ? (
        <div>
          <b>订阅事件</b>
          <Space size={4} wrap>{c.eventsubs.map((e) => <Tag key={e.event} color="green" style={{ margin: 0 }}>{e.event}</Tag>)}</Space>
        </div>
      ) : null}
      {c?.eventpubs?.length ? (
        <div>
          <b>发布事件</b>
          <Space size={4} wrap>{c.eventpubs.map((e) => <Tag key={e.event} color="gold" style={{ margin: 0 }}>{e.event}</Tag>)}</Space>
        </div>
      ) : null}
      {!c || (!c.menus?.length && !c.pages?.length && !c.routes?.length && !c.widgets?.length && !c.services?.length && !c.eventsubs?.length && !c.eventpubs?.length) ? (
        <Empty description="该插件未贡献任何扩展点" />
      ) : null}
    </div>
  );

  const manifestTab = (
    <pre style={{ background: 'rgba(0,0,0,0.04)', padding: 12, borderRadius: 6, maxHeight: 480, overflow: 'auto', fontSize: 12 }}>
      {detail?.manifest ? JSON.stringify(detail.manifest, null, 2) : '加载中…'}
    </pre>
  );

  return (
    <Drawer title={`插件详情 - ${plugin?.name}`} open={open} onClose={onClose} width={620}>
      <Tabs
        items={[
          { key: 'overview', label: '概览', children: overview },
          { key: 'logs', label: '日志', children: logsTab },
          { key: 'contributions', label: '贡献点', children: contributions },
          { key: 'manifest', label: 'Manifest', children: manifestTab },
        ]}
      />
    </Drawer>
  );
}

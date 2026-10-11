import { useState } from 'react';
import { Card, Button, Table, Tag, Space, message, Alert, Descriptions, Modal, Tooltip } from 'antd';
import { SafetyOutlined, ThunderboltOutlined } from '@ant-design/icons';
import api from '../../services/client';

interface Vuln { package: string; version: string; vuln_ids: string[]; candidate?: string; upgradable?: boolean; }
interface KernelStatus { current: string; latest: string; needs_reboot: boolean; }

export default function Vulnerabilities() {
  const [vulns, setVulns] = useState<Vuln[]>([]);
  const [scanning, setScanning] = useState(false);
  const [kernel, setKernel] = useState<KernelStatus | null>(null);
  const [upgradable, setUpgradable] = useState<number | null>(null);
  const [upgrading, setUpgrading] = useState(false);

  const scan = async () => {
    setScanning(true);
    try {
      const [vRes, kRes, uRes] = await Promise.all([
        api.post('/security/cve/scan'),
        api.get('/security/cve/kernel'),
        api.get('/security/cve/upgradable'),
      ]);
      setVulns(vRes.data.data?.vulnerabilities || []);
      setKernel(kRes.data.data);
      setUpgradable(uRes.data.data?.count ?? 0);
      message.success(`扫描完成，发现 ${vRes.data.data?.count || 0} 个有漏洞的包`);
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '扫描失败');
    } finally {
      setScanning(false);
    }
  };

  const showNoFix = (noFix: string[]) => {
    if (!noFix?.length) return;
    Modal.info({
      title: `以下 ${noFix.length} 个包 Debian 源中暂无修复版本`,
      width: 520,
      content: (
        <ul style={{ margin: '12px 0 0', paddingLeft: 20, maxHeight: 300, overflow: 'auto' }}>
          {noFix.map((p) => <li key={p}>{p}</li>)}
        </ul>
      ),
    });
  };

  const upgradeAll = async () => {
    const fixable = vulns.filter((v) => v.upgradable);
    if (fixable.length === 0) {
      message.warning('所有漏洞包在 Debian 源中均暂无修复版本，无法通过升级清除');
      return;
    }
    setUpgrading(true);
    try {
      const res = await api.post('/security/cve/upgrade', { packages: fixable.map((v) => v.package) });
      message.success(res.data.data?.message || '升级完成');
      showNoFix(res.data.data?.no_fix || []);
      scan();
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '升级失败');
    } finally {
      setUpgrading(false);
    }
  };

  const upgradeOne = async (pkg: string) => {
    try {
      const res = await api.post('/security/cve/upgrade', { packages: [pkg] });
      message.success(`${pkg}: ${res.data.data?.message || '升级完成'}`);
      showNoFix(res.data.data?.no_fix || []);
      scan();
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '升级失败');
    }
  };

  return (
    <Space direction="vertical" style={{ width: '100%' }} size="middle">
      <Card title={<span><SafetyOutlined /> 系统漏洞扫描</span>} extra={<Button type="primary" icon={<ThunderboltOutlined />} onClick={scan} loading={scanning}>立即扫描</Button>}>
        <Alert message="扫描已安装的 apt 软件包，通过 osv.dev 查询已知 CVE 漏洞（在线查询，可能耗时 1-2 分钟）" type="info" showIcon style={{ marginBottom: 16 }} />
        {kernel && (
          <Descriptions size="small" column={3} bordered style={{ marginBottom: 16 }}>
            <Descriptions.Item label="当前内核">{kernel.current}</Descriptions.Item>
            <Descriptions.Item label="最新已装内核">{kernel.latest}</Descriptions.Item>
            <Descriptions.Item label="需重启">{kernel.needs_reboot ? <Tag color="red">是</Tag> : <Tag color="green">否</Tag>}</Descriptions.Item>
          </Descriptions>
        )}
        {upgradable !== null && (
          <Alert
            message={`${upgradable} 个包可升级${vulns.length > 0 ? `，其中 ${vulns.length} 个有漏洞` : ''}`}
            type={upgradable > 0 ? 'warning' : 'success'} showIcon style={{ marginBottom: 16 }}
            action={vulns.length > 0 ? <Button size="small" onClick={upgradeAll} loading={upgrading}>一键升级漏洞包</Button> : undefined}
          />
        )}
      </Card>

      {vulns.length > 0 && (
        <Card title={`有漏洞的包 (${vulns.length})`}>
          <Table size="small" dataSource={vulns} rowKey="package" pagination={{ pageSize: 20 }}
            columns={[
              { title: '包名', dataIndex: 'package', key: 'package' },
              { title: '当前版本', dataIndex: 'version', key: 'version' },
              { title: 'CVE 数', key: 'count', width: 80, render: (_: unknown, r: Vuln) => <Tag color="red">{r.vuln_ids.length}</Tag> },
              { title: 'CVE', key: 'vulns', ellipsis: true, render: (_: unknown, r: Vuln) => <Space wrap>{r.vuln_ids.map((id) => <Tag key={id}>{id}</Tag>)}</Space> },
              { title: '可修复', key: 'fixable', width: 90, render: (_: unknown, r: Vuln) => r.upgradable
                ? <Tag color="green" style={{ margin: 0 }}>可升级</Tag>
                : <Tooltip title={`Debian 源中暂无修复版本${r.candidate ? `（候选 ${r.candidate}）` : ''}`}><Tag color="default" style={{ margin: 0 }}>暂无修复</Tag></Tooltip> },
              { title: '操作', key: 'action', width: 80, render: (_: unknown, r: Vuln) => r.upgradable
                ? <Button size="small" type="link" onClick={() => upgradeOne(r.package)}>升级</Button>
                : <Tooltip title="Debian 源中暂无修复版本"><Button size="small" type="link" disabled>升级</Button></Tooltip> },
            ]}
          />
        </Card>
      )}
    </Space>
  );
}

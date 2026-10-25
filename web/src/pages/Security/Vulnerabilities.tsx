import { useEffect, useState } from 'react';
import { Card, Button, Table, Tag, Space, message, Alert, Descriptions, Modal, Tooltip } from 'antd';
import { SafetyOutlined, ThunderboltOutlined } from '@ant-design/icons';
import api from '../../services/client';
import './Vulnerabilities.css';

interface Vuln { package: string; version: string; vuln_ids: string[]; candidate?: string; upgradable?: boolean; }
interface KernelStatus { current: string; latest: string; needs_reboot: boolean; }

// 扫描期间的耗时计数（秒）
function useElapsed(active: boolean) {
  const [sec, setSec] = useState(0);
  useEffect(() => {
    if (!active) return;
    setSec(0);
    const t = setInterval(() => setSec((s) => s + 1), 1000);
    return () => clearInterval(t);
  }, [active]);
  return sec;
}

// 数字滚动：目标值变化时以缓出曲线滚到目标
function useCountUp(target: number, duration = 600) {
  const [val, setVal] = useState(0);
  useEffect(() => {
    if (target === 0) { setVal(0); return; }
    let raf = 0;
    const start = performance.now();
    const tick = (now: number) => {
      const p = Math.min(1, (now - start) / duration);
      setVal(Math.round(target * (1 - Math.pow(1 - p, 3))));
      if (p < 1) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [target, duration]);
  return val;
}

const SCAN_STAGES = ['读取已安装软件包…', '正在查询 osv.dev 漏洞库（在线）…', '汇总分析扫描结果…'];

// 扫描（osv.dev 在线查询）与升级（apt update+install）都是分钟级操作：
// axios 实例默认 30s 超时对它们必然误杀，单独放宽到 5 分钟。
const SLOW_OP_TIMEOUT = 300000;

export default function Vulnerabilities() {
  const [vulns, setVulns] = useState<Vuln[]>([]);
  const [scanning, setScanning] = useState(false);
  const [kernel, setKernel] = useState<KernelStatus | null>(null);
  const [upgradable, setUpgradable] = useState<number | null>(null);
  const [upgrading, setUpgrading] = useState(false);
  const [upgradingPkg, setUpgradingPkg] = useState(''); // 单包升级中的包名（按钮 loading）
  const [scanSeq, setScanSeq] = useState(0); // 触发结果区入场动画

  const elapsed = useElapsed(scanning);
  const vulnCount = useCountUp(vulns.length);
  const stage = elapsed < 4 ? SCAN_STAGES[0] : elapsed < 30 ? SCAN_STAGES[1] : SCAN_STAGES[2];

  const scan = async () => {
    setScanning(true);
    try {
      const [vRes, kRes, uRes] = await Promise.all([
        api.post('/security/cve/scan', null, { timeout: SLOW_OP_TIMEOUT }),
        api.get('/security/cve/kernel'),
        api.get('/security/cve/upgradable'),
      ]);
      setVulns(vRes.data.data?.vulnerabilities || []);
      setKernel(kRes.data.data);
      setUpgradable(uRes.data.data?.count ?? 0);
      setScanSeq((n) => n + 1);
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
    const hide = message.loading('正在升级漏洞包（刷新源 + 安装，通常需要 30 秒至 2 分钟）…', 0);
    try {
      const res = await api.post('/security/cve/upgrade', { packages: fixable.map((v) => v.package) }, { timeout: SLOW_OP_TIMEOUT });
      hide();
      message.success(res.data.data?.message || '升级完成');
      showNoFix(res.data.data?.no_fix || []);
      scan();
    } catch (e: unknown) {
      hide();
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || (e as Error)?.message || '升级失败');
    } finally {
      setUpgrading(false);
    }
  };

  const upgradeOne = async (pkg: string) => {
    setUpgradingPkg(pkg);
    const hide = message.loading(`正在升级 ${pkg}（通常需要 30 秒至 2 分钟）…`, 0);
    try {
      const res = await api.post('/security/cve/upgrade', { packages: [pkg] }, { timeout: SLOW_OP_TIMEOUT });
      hide();
      message.success(`${pkg}: ${res.data.data?.message || '升级完成'}`);
      showNoFix(res.data.data?.no_fix || []);
      scan();
    } catch (e: unknown) {
      hide();
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || (e as Error)?.message || '升级失败');
    } finally {
      setUpgradingPkg('');
    }
  };

  return (
    <Space direction="vertical" style={{ width: '100%' }} size="middle">
      <Card title={<span><SafetyOutlined /> 系统漏洞扫描</span>} extra={<Button type="primary" icon={<ThunderboltOutlined />} onClick={scan} loading={scanning}>立即扫描</Button>}>
        <Alert message="扫描已安装的 apt 软件包，通过 osv.dev 查询已知 CVE 漏洞（在线查询，可能耗时 1-2 分钟）" type="info" showIcon style={{ marginBottom: 16 }} />
        {scanning && (
          <div className="scan-anim" style={{ marginBottom: 16 }}>
            <span className="scan-radar">
              <i className="scan-ring" />
              <i className="scan-ring scan-ring-delay" />
              <SafetyOutlined className="scan-radar-icon" />
            </span>
            <div className="scan-anim-text">
              <span className="scan-stage">{stage}<span className="scan-dots"><i /><i /><i /></span></span>
              <span className="scan-elapsed">已耗时 {elapsed} 秒</span>
            </div>
            <div className="scan-bar" style={{ flex: 1, marginTop: 0 }}><i /></div>
          </div>
        )}
        {kernel && (
          <Descriptions size="small" column={3} bordered style={{ marginBottom: 16 }}>
            <Descriptions.Item label="当前内核">{kernel.current}</Descriptions.Item>
            <Descriptions.Item label="最新已装内核">{kernel.latest}</Descriptions.Item>
            <Descriptions.Item label="需重启">{kernel.needs_reboot ? <Tag color="red">是</Tag> : <Tag color="green">否</Tag>}</Descriptions.Item>
          </Descriptions>
        )}
        {upgrading && <div className="scan-bar"><i /></div>}
        {upgradable !== null && (
          <Alert
            message={`${upgradable} 个包可升级${vulns.length > 0 ? `，其中 ${vulnCount} 个有漏洞` : ''}`}
            type={upgradable > 0 ? 'warning' : 'success'} showIcon style={{ marginBottom: 0 }}
            action={vulns.length > 0 ? <Button size="small" onClick={upgradeAll} loading={upgrading}>一键升级漏洞包</Button> : undefined}
          />
        )}
      </Card>

      {vulns.length > 0 && (
        <Card
          key={scanSeq}
          className="fade-in-up"
          title={`有漏洞的包 (${vulnCount})`}
        >
          <Table size="small" dataSource={vulns} rowKey="package" pagination={{ pageSize: 20 }}
            rowClassName={() => 'vuln-row'}
            onRow={(_, i) => ({ style: { '--row-i': i } as React.CSSProperties })}
            columns={[
              { title: '包名', dataIndex: 'package', key: 'package' },
              { title: '当前版本', dataIndex: 'version', key: 'version' },
              { title: 'CVE 数', key: 'count', width: 80, render: (_: unknown, r: Vuln) => <Tag color="red">{r.vuln_ids.length}</Tag> },
              { title: 'CVE', key: 'vulns', ellipsis: true, render: (_: unknown, r: Vuln) => <Space wrap>{r.vuln_ids.map((id) => <Tag key={id}>{id}</Tag>)}</Space> },
              { title: '可修复', key: 'fixable', width: 90, render: (_: unknown, r: Vuln) => r.upgradable
                ? <Tag color="green" style={{ margin: 0 }}>可升级</Tag>
                : <Tooltip title={`Debian 源中暂无修复版本${r.candidate ? `（候选 ${r.candidate}）` : ''}`}><Tag color="default" style={{ margin: 0 }}>暂无修复</Tag></Tooltip> },
              { title: '操作', key: 'action', width: 80, render: (_: unknown, r: Vuln) => r.upgradable
                ? <Button size="small" type="link" loading={upgradingPkg === r.package} disabled={!!upgradingPkg || upgrading} onClick={() => upgradeOne(r.package)}>升级</Button>
                : <Tooltip title="Debian 源中暂无修复版本"><span><Button size="small" type="link" disabled>升级</Button></span></Tooltip> },
            ]}
          />
        </Card>
      )}
    </Space>
  );
}

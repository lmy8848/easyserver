import { useEffect, useState } from 'react';
import { Select, Tag, Space } from 'antd';
import api from '../services/api';
import { getRuntimeIcon } from '../pages/Runtime/types';

interface RuntimeVersionOption {
  id: number;
  name: string;       // lang: node / python / ...
  version: string;    // exact: 20.11.0
  status: string;     // installed / installing / failed / ...
  is_default: boolean;
}

interface RuntimeVersionSelectProps {
  value?: number;
  onChange?: (v: number | undefined) => void;
  /** Show non-installed rows as disabled options so users see why they can't pick them. */
  showDisabled?: boolean;
}

// RuntimeVersionSelect lists every runtime_version row from GET /runtime;
// only status='installed' rows are selectable. AC3 wants installing/failed
// to be visible-but-disabled so the user can tell "node 22 is missing"
// from "node 22 doesn't exist yet". Fetch is one-shot per mount — the
// list is tiny (≤ ~20 rows) and the parent Modal mounts on each open.
export default function RuntimeVersionSelect({ value, onChange, showDisabled = true }: RuntimeVersionSelectProps) {
  const [envs, setEnvs] = useState<RuntimeVersionOption[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    setLoading(true);
    api.get('/runtime')
      .then(res => setEnvs(res.data.data?.environments || []))
      .catch(() => setEnvs([]))
      .finally(() => setLoading(false));
  }, []);

  const STATUS_LABEL: Record<string, { color: string; label: string }> = {
    installed: { color: 'green', label: '已安装' },
    installing: { color: 'blue', label: '安装中' },
    failed: { color: 'red', label: '失败' },
    uninstalling: { color: 'orange', label: '卸载中' },
    uninstall_failed: { color: 'red', label: '卸载失败' },
  };

  // Hide 'uninstalled' terminal rows — they're not actionable here, just
  // clutter from the runtime list page. installing/failed stay (greyed out)
  // because seeing "node 22 is still installing" tells a different story
  // from "node 22 doesn't exist yet".
  const selectable = envs.filter(e => e.status !== 'uninstalled');
  const visible = showDisabled ? selectable : selectable.filter(e => e.status === 'installed');
  const options = visible.map(e => {
    const meta = STATUS_LABEL[e.status] ?? { color: 'default', label: e.status };
    return {
      value: e.id,
      disabled: e.status !== 'installed',
      label: (
        <Space>
          <span>{getRuntimeIcon(e.name)}</span>
          <span>{e.name} {e.version}</span>
          {e.is_default && <Tag color="blue" style={{ fontSize: 10 }}>默认</Tag>}
          {e.status !== 'installed' && <Tag color={meta.color} style={{ fontSize: 10 }}>{meta.label}</Tag>}
        </Space>
      ),
    };
  });

  return (
    <Select
      value={value}
      onChange={onChange}
      loading={loading}
      placeholder="选择已安装的运行时版本"
      options={options}
      showSearch
      filterOption={(input, option) => {
        const env = visible.find(e => e.id === option?.value);
        if (!env) return false;
        const q = input.toLowerCase();
        return env.name.includes(q) || env.version.includes(q);
      }}
      notFoundContent={loading ? '加载中...' : '没有已安装的运行时，请先到「运行环境管理」安装'}
    />
  );
}

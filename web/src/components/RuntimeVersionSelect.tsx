import { useEffect, useState } from 'react';
import { Select, Space } from 'antd';
import api from '../services/client';
import { getRuntimeIcon } from '../pages/Runtime/types';

interface RuntimeVersionOption {
  name: string;    // lang: node / python / ...
  version: string; // exact: 20.11.0
}

interface RuntimeVersionSelectProps {
  /** 受控值：lang@exact 字符串（ADR-0009 绑定键）。"" 或 undefined 表示未选。 */
  value?: string;
  /** 回传 lang@exact（如 "node@20.11.0"），表单存 runtime 字段。 */
  onChange?: (v: string | undefined) => void;
}

// RuntimeVersionSelect 从 GET /runtime 拉取已安装环境（installs/ 目录扫描，
// ADR-0009 后只返回 installed 行，无 installing/failed 过程态）。value/onChange
// 直接传 lang@exact 字符串——它就是绑定键，无需 id 间接层。
export default function RuntimeVersionSelect({ value, onChange }: RuntimeVersionSelectProps) {
  const [envs, setEnvs] = useState<RuntimeVersionOption[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    setLoading(true);
    api.get('/runtime')
      .then(res => setEnvs(res.data.data?.environments || []))
      .catch(() => setEnvs([]))
      .finally(() => setLoading(false));
  }, []);

  const options = envs.map(e => ({
    value: `${e.name}@${e.version}`,
    label: (
      <Space>
        <span>{getRuntimeIcon(e.name)}</span>
        <span>{e.name} {e.version}</span>
      </Space>
    ),
  }));

  return (
    <Select
      allowClear
      value={value || undefined}
      onChange={(v?: string) => onChange?.(v || undefined)}
      loading={loading}
      placeholder="选择已安装的运行时版本"
      options={options}
      showSearch
      filterOption={(input, option) => {
        const q = input.toLowerCase();
        const v = String(option?.value || '');
        return v.toLowerCase().includes(q);
      }}
      notFoundContent={loading ? '加载中...' : '没有已安装的运行时，请先到「运行环境管理」安装'}
    />
  );
}

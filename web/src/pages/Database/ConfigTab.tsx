import { useState } from 'react';
import {
  Row, Col, Input, InputNumber, Select, Tag, Empty, Spin,
} from 'antd';
import type { ConfigTabProps } from './types';

const { TextArea } = Input;

// 配置 tab — 结构化参数编辑。切到本 tab 时父组件自动加载配置；保存/刷新按钮
// 在父组件 tab 栏右侧（tabBarExtraContent，见 index.tsx），本组件只渲染参数
// 表单，参数网格化排列（多行值如 Redis save 独占一行）。配置是单段结构：params
// （引擎当前值）+ meta（渲染元数据）。输入控件按 type/unit 区分：带字节单位 →
// InputNumber + 单位下拉，有 options → Select，含换行 → TextArea，否则文本输入。
//
// 字节单位语义：MySQL 内存参数 / Redis maxmemory 引擎返回字节（读到啥返回啥），
// 但 UI 让用户以 KB/MB/GB 输入——保存时按所选单位换算成字节传给后端（裸字面量）。
// PG 内存参数是 string 型（ALTER SYSTEM 要带单位引号串），走普通文本输入。

// SIZE_FACTOR: 单位 → 字节倍数（1024 进制）。
const SIZE_FACTOR: Record<string, number> = {
  B: 1,
  KB: 1024,
  MB: 1024 ** 2,
  GB: 1024 ** 3,
  TB: 1024 ** 4,
};
const SIZE_UNITS = Object.keys(SIZE_FACTOR);
// number 型 + 字节单位 token → 渲染单位下拉。
const isSizeParam = (param: any) =>
  param.type === 'number' && !!param.unit && param.unit.toUpperCase() in SIZE_FACTOR;
// 单位 → 字节倍数（unknown 兜底 1，防御脏数据）。
const factorOf = (unit: string) => SIZE_FACTOR[unit] ?? 1;

export default function ConfigTab({
  server, dbConfig, dbConfigLoading, onUpdateDBParam,
}: ConfigTabProps) {
  // 每个 size 参数当前选中的单位（默认取 param.unit，用户可切）。
  const [units, setUnits] = useState<Record<string, string>>({});
  const unitOf = (key: string, def: string) => units[key] || def.toUpperCase();

  // 引擎字节值 → 当前单位下的显示值（数值都来自 SET 过的整数，Math.round 精确）。
  const bytesToDisplay = (bytes: string | undefined, unit: string) => {
    const n = Number(bytes);
    if (!bytes || !Number.isFinite(n)) return '';
    return String(Math.round(n / factorOf(unit)));
  };
  // 显示值 → 字节（保存时按单位换算后传给后端）。
  const displayToBytes = (display: string | null | undefined, unit: string) => {
    if (display == null || display === '') return '';
    const n = Number(display);
    if (!Number.isFinite(n)) return '';
    return String(Math.round(n * factorOf(unit)));
  };

  if (dbConfigLoading && !dbConfig) {
    return <div style={{ textAlign: 'center', padding: 40 }}><Spin /></div>;
  }
  if (!dbConfig) {
    return <Empty description={`读取 ${server.display_name} 配置失败，点击右上角刷新重试`} />;
  }

  const params = dbConfig.config?.params || {};
  const meta = dbConfig.config?.meta || [];
  // 含换行的值（如 Redis save 多行策略）用多行输入框且独占一行。
  const isMultiline = (key: string) => (params[key] || '').includes('\n');

  const renderInput = (param: any) => {
    if (param.options?.length) {
      return (
        <Select
          value={params[param.key] || ''}
          onChange={(val) => onUpdateDBParam(param.key, val)}
          style={{ width: '100%' }}
        >
          {(param.options || []).map((opt: string) => (
            <Select.Option key={opt} value={opt}>{opt}</Select.Option>
          ))}
        </Select>
      );
    }
    if (isSizeParam(param)) {
      const unit = unitOf(param.key, param.unit);
      return (
        <div style={{ display: 'flex', gap: 8 }}>
          <InputNumber
            stringMode
            value={bytesToDisplay(params[param.key], unit)}
            onChange={(val) => onUpdateDBParam(param.key, displayToBytes(val, unit))}
            style={{ width: '100%' }}
          />
          <Select
            value={unit}
            onChange={(u) => setUnits(prev => ({ ...prev, [param.key]: u }))}
            style={{ width: 80 }}
          >
            {SIZE_UNITS.map(u => (
              <Select.Option key={u} value={u}>{u}</Select.Option>
            ))}
          </Select>
        </div>
      );
    }
    if (param.type === 'number') {
      return (
        <InputNumber
          stringMode
          value={params[param.key]}
          onChange={(val) => onUpdateDBParam(param.key, val == null ? '' : String(val))}
          style={{ width: '100%' }}
        />
      );
    }
    if (isMultiline(param.key)) {
      return (
        <TextArea
          value={params[param.key] || ''}
          onChange={(e) => onUpdateDBParam(param.key, e.target.value)}
          rows={3}
          style={{ width: '100%' }}
        />
      );
    }
    return (
      <Input
        value={params[param.key] || ''}
        onChange={(e) => onUpdateDBParam(param.key, e.target.value)}
      />
    );
  };

  return (
    <div>
      <Row gutter={[24, 20]}>
        {meta.map((param: any) => (
          <Col key={param.key} xs={24} sm={12} lg={isMultiline(param.key) ? 24 : 8}>
            <div style={{ marginBottom: 4 }}>
              <strong>{param.label}</strong>
              <span style={{ color: '#8c8c8c', marginLeft: 8, fontSize: 12 }}>{param.key}</span>
              {param.unit && <Tag style={{ marginLeft: 8 }}>{param.unit}</Tag>}
            </div>
            <div style={{ color: '#666', fontSize: 12, marginBottom: 4 }}>{param.description}</div>
            {renderInput(param)}
          </Col>
        ))}
      </Row>
    </div>
  );
}

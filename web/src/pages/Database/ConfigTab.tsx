import { useState } from 'react';
import {
  Row, Col, Input, InputNumber, Select, Empty, Spin,
} from 'antd';
import type { ConfigTabProps } from './types';

const { TextArea } = Input;

// 配置 tab — 结构化参数编辑。切到本 tab 时父组件自动加载配置；保存/刷新按钮
// 在父组件 tab 栏右侧（tabBarExtraContent，见 index.tsx），本组件只渲染参数
// 表单，参数网格化排列（多行值如 Redis save 独占一行）。配置是单段结构：params
// （引擎当前值）+ meta（渲染元数据）。输入控件统一用 InputNumber + addonAfter：
// 可切换单位（字节类 / PG 单位串）→ addonAfter 单位下拉；单一单位（如"秒"）→
// addonAfter 固定文本；有 options → Select，含换行 → TextArea，否则文本输入。
//
// 单位语义分两种：
// - 字节换算（MySQL 内存参数 / Redis maxmemory）：引擎返回字节，UI 按单位显示，
//   保存时换算回字节（裸字面量 SET）。
// - 字符串拼接（PG 内存参数）：引擎返回带单位串（'128MB'），UI 解析数字 + 单位，
//   保存时拼回带单位串（ALTER SYSTEM 要引号串，不能裸字节换算）。
// PG 的秒/个等无单位数字仍走 number 分支。

// SIZE_FACTOR: 单位 → 字节倍数（1024 进制）。
const SIZE_FACTOR: Record<string, number> = {
  B: 1,
  KB: 1024,
  MB: 1024 ** 2,
  GB: 1024 ** 3,
  TB: 1024 ** 4,
};
const SIZE_UNITS = Object.keys(SIZE_FACTOR);
// 字节换算参数（number 型 + 字节单位 token）→ 显示/保存都换算字节。
const isSizeParam = (param: any) =>
  param.type === 'number' && !!param.unit && param.unit.toUpperCase() in SIZE_FACTOR;
// PG 字符串单位参数（string 型 + 单位 token）→ 解析/拼接带单位串。
const isStrUnitParam = (param: any) =>
  param.type === 'string' && !!param.unit && param.unit.toUpperCase() in SIZE_FACTOR;
// 单位 → 字节倍数（unknown 兜底 1，防御脏数据）。
const factorOf = (unit: string) => SIZE_FACTOR[unit] ?? 1;

export default function ConfigTab({
  server, dbConfig, dbConfigLoading, onUpdateDBParam,
}: ConfigTabProps) {
  // 每个可切换单位参数当前选中的单位（默认取 param.unit，用户可切）。
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
  // PG 带单位串 "128MB" → 数字部分 "128"。解析不了（无单位）→ 原样，保存也原样。
  const strToNum = (v: string | undefined) => {
    if (!v) return '';
    const m = /^(\d+)\s*([kKmMgGtT][bB])?$/.exec(v.trim());
    return m ? m[1] : v;
  };
  // PG 数字 + 单位 → 带单位串（保存传给后端 ALTER SYSTEM）。
  const numToStr = (num: string | null | undefined, unit: string) => {
    if (num == null || num === '') return '';
    const n = Number(num);
    if (!Number.isFinite(n)) return '';
    return `${Math.round(n)}${unit}`;
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
        // 可切换单位 → addonAfter 内嵌 borderless Select。addonAfter 内部是
        // inline-flex 内容宽，靠 rootClassName + index.css 的 .config-unit-input
        // 把外层改回 flex 撑满（见 index.css），保证与其他输入框等宽。
        <InputNumber
          stringMode
          value={bytesToDisplay(params[param.key], unit)}
          onChange={(val) => onUpdateDBParam(param.key, displayToBytes(val, unit))}
          style={{ width: '100%' }}
          rootClassName="config-unit-input"
          addonAfter={(
            <Select
              value={unit}
              onChange={(u) => setUnits(prev => ({ ...prev, [param.key]: u }))}
              variant="borderless"
              style={{ width: 76 }}
            >
              {SIZE_UNITS.map(u => (
                <Select.Option key={u} value={u}>{u}</Select.Option>
              ))}
            </Select>
          )}
        />
      );
    }
    if (isStrUnitParam(param)) {
      // PG 字符串单位参数：值 "128MB" → 数字部分显示，单位可切；保存拼回 "128MB"。
      // 初始单位优先取值里的实际单位（如 "1GB" 显示 GB），其次 param.unit。
      const v = params[param.key] || '';
      const vm = /^(\d+)\s*([kKmMgGtT][bB])?$/.exec(v.trim());
      const unit = units[param.key] || (vm && vm[2] ? vm[2].toUpperCase() : param.unit.toUpperCase());
      return (
        <InputNumber
          stringMode
          value={strToNum(v)}
          onChange={(val) => onUpdateDBParam(param.key, numToStr(val, unit))}
          style={{ width: '100%' }}
          rootClassName="config-unit-input"
          addonAfter={(
            <Select
              value={unit}
              onChange={(u) => setUnits(prev => ({ ...prev, [param.key]: u }))}
              variant="borderless"
              style={{ width: 76 }}
            >
              {SIZE_UNITS.map(u => (
                <Select.Option key={u} value={u}>{u}</Select.Option>
              ))}
            </Select>
          )}
        />
      );
    }
    if (param.type === 'number') {
      // 单一单位参数（如 wait_timeout "秒"）→ addonAfter 固定单位文本（不切换）；
      // 无单位 → 纯数字。
      if (param.unit) {
        return (
          <InputNumber
            stringMode
            value={params[param.key]}
            onChange={(val) => onUpdateDBParam(param.key, val == null ? '' : String(val))}
            style={{ width: '100%' }}
            rootClassName="config-unit-input"
            addonAfter={<span style={{ color: '#666' }}>{param.unit}</span>}
          />
        );
      }
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
          <Col key={param.key} xs={24} sm={12} lg={isMultiline(param.key) ? 24 : 6}>
            <div style={{ marginBottom: 4 }}>
              <strong>{param.label}</strong>
              <span style={{ color: '#8c8c8c', marginLeft: 8, fontSize: 12 }}>{param.key}</span>
            </div>
            <div style={{ color: '#666', fontSize: 12, marginBottom: 4 }}>{param.description}</div>
            {renderInput(param)}
          </Col>
        ))}
      </Row>
    </div>
  );
}

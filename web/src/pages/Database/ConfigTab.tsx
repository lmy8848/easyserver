import {
  Row, Col, Input, Select, Tag, Empty, Spin,
} from 'antd';
import type { ConfigTabProps } from './types';

const { TextArea } = Input;

// 配置 tab — 结构化参数编辑。切到本 tab 时父组件自动加载配置；保存/刷新按钮
// 在父组件 tab 栏右侧（tabBarExtraContent，见 index.tsx），本组件只渲染参数
// 表单，参数网格化排列（多行值如 Redis save 独占一行）。配置是单段结构：params
// （引擎当前值）+ meta（渲染元数据）。
export default function ConfigTab({
  server, dbConfig, dbConfigLoading, onUpdateDBParam,
}: ConfigTabProps) {
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
            {param.options?.length ? (
              <Select
                value={params[param.key] || ''}
                onChange={(val) => onUpdateDBParam(param.key, val)}
                style={{ width: '100%' }}
              >
                {(param.options || []).map((opt: string) => (
                  <Select.Option key={opt} value={opt}>{opt}</Select.Option>
                ))}
              </Select>
            ) : isMultiline(param.key) ? (
              <TextArea
                value={params[param.key] || ''}
                onChange={(e) => onUpdateDBParam(param.key, e.target.value)}
                rows={3}
                style={{ width: '100%' }}
              />
            ) : (
              <Input
                value={params[param.key] || ''}
                onChange={(e) => onUpdateDBParam(param.key, e.target.value)}
              />
            )}
          </Col>
        ))}
      </Row>
    </div>
  );
}

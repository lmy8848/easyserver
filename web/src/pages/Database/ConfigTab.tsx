import {
  Button, Space, Tag, Input, InputNumber, Select, Tabs, Empty,
} from 'antd';
import { CodeOutlined, ReloadOutlined } from '@ant-design/icons';
import type { ConfigTabProps } from './types';

// 配置文件 tab — 结构化参数编辑，保持独立 tab。
export default function ConfigTab({
  server, dbConfig, dbConfigLoading, busy,
  onFetchDBConfig, onSaveDBConfig, onUpdateDBParam,
}: ConfigTabProps) {
  return (
    <div>
      {!dbConfig ? (
        <div style={{ textAlign: 'center', padding: 40 }}>
          <Button type="primary" icon={<CodeOutlined />} loading={dbConfigLoading}
            onClick={onFetchDBConfig}>加载配置</Button>
          <p style={{ color: '#999', marginTop: 12 }}>读取当前数据库实例容器内的配置文件</p>
        </div>
      ) : dbConfig.found === false ? (
        <Empty description={`未找到 ${server.display_name} 配置文件`} />
      ) : (
        <div>
          <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Space>
              <Tag>{dbConfig.config?.file_path}</Tag>
              <span style={{ color: '#8c8c8c', fontSize: 12 }}>修改后需重启 {server.display_name} 生效</span>
            </Space>
            <Space>
              <Button icon={<ReloadOutlined />} onClick={onFetchDBConfig}>重新加载</Button>
              <Button type="primary" loading={busy === 'save-config'} onClick={onSaveDBConfig}>保存配置</Button>
            </Space>
          </div>
          <Tabs
            type="card"
            items={(dbConfig.config?.sections || []).map((section: any) => ({
              key: section.name,
              label: section.name === 'main' ? server.display_name : `[${section.name}]`,
              children: (
                <div>
                  {(dbConfig.sections?.[section.name]?.meta || []).map((param: any) => (
                    <div key={param.key} style={{ marginBottom: 16 }}>
                      <div style={{ marginBottom: 4 }}>
                        <strong>{param.label}</strong>
                        <span style={{ color: '#8c8c8c', marginLeft: 8, fontSize: 12 }}>{param.key}</span>
                        {param.unit && <Tag style={{ marginLeft: 8 }}>{param.unit}</Tag>}
                      </div>
                      <div style={{ color: '#666', fontSize: 12, marginBottom: 4 }}>{param.description}</div>
                      {param.type === 'select' ? (
                        <Select
                          value={section.params?.[param.key] || param.default}
                          onChange={(val) => onUpdateDBParam(section.name, param.key, val)}
                          style={{ width: 300 }}
                        >
                          {(param.options || []).map((opt: string) => (
                            <Select.Option key={opt} value={opt}>{opt}</Select.Option>
                          ))}
                        </Select>
                      ) : param.type === 'number' ? (
                        <InputNumber
                          value={Number(section.params?.[param.key]) || Number(param.default)}
                          onChange={(val) => onUpdateDBParam(section.name, param.key, String(val || ''))}
                          style={{ width: 300 }}
                        />
                      ) : (
                        <Input
                          value={section.params?.[param.key] || ''}
                          onChange={(e) => onUpdateDBParam(section.name, param.key, e.target.value)}
                          placeholder={param.default}
                          style={{ width: 400 }}
                        />
                      )}
                    </div>
                  ))}
                  {/* Show extra params not in common list */}
                  {Object.entries(section.params || {}).filter(([key]) =>
                    !(dbConfig.sections?.[section.name]?.meta || []).some((m: any) => m.key === key)
                  ).map(([key, value]) => (
                    <div key={key} style={{ marginBottom: 16 }}>
                      <div style={{ marginBottom: 4 }}>
                        <Tag color="default">{key}</Tag>
                      </div>
                      <Input
                        value={value as string}
                        onChange={(e) => onUpdateDBParam(section.name, key, e.target.value)}
                        style={{ width: 400 }}
                      />
                    </div>
                  ))}
                </div>
              ),
            }))}
          />
        </div>
      )}
    </div>
  );
}

import { Drawer, Collapse, Spin, Empty } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { CronDoc } from '../../types';
import { MARKDOWN_STYLES } from './types';

interface CronDocsProps {
  visible: boolean;
  docs: CronDoc[];
  loading: boolean;
  onClose: () => void;
}

export default function CronDocs({ visible, docs, loading, onClose }: CronDocsProps) {
  return (
    <Drawer
      title={<span><QuestionCircleOutlined /> Cron 表达式手册</span>}
      open={visible}
      onClose={onClose}
      size={600}
    >
      {loading ? (
        <div style={{ textAlign: 'center', padding: 40 }}><Spin /></div>
      ) : docs.length > 0 ? (
        <Collapse
          defaultActiveKey={docs.map(d => String(d.id))}
          ghost
          items={docs.map(doc => ({
            key: String(doc.id),
            label: <strong>{doc.title}</strong>,
            children: (
              <div style={{ fontSize: 14, lineHeight: 1.8 }}>
                <Markdown
                  remarkPlugins={[remarkGfm]}
                  components={{
                    table: ({children}) => <table style={MARKDOWN_STYLES.table}>{children}</table>,
                    th: ({children}) => <th style={MARKDOWN_STYLES.th}>{children}</th>,
                    td: ({children}) => <td style={MARKDOWN_STYLES.td}>{children}</td>,
                    code: ({children, className}) => {
                      const isInline = !className;
                      return isInline
                        ? <code style={MARKDOWN_STYLES.code}>{children}</code>
                        : <code style={{...MARKDOWN_STYLES.code, display: 'block', padding: 16}}>{children}</code>;
                    },
                    pre: ({children}) => <pre style={MARKDOWN_STYLES.pre}>{children}</pre>,
                  }}
                >
                  {doc.content}
                </Markdown>
              </div>
            ),
          }))}
        />
      ) : (
        <Empty description="暂无文档" />
      )}
    </Drawer>
  );
}

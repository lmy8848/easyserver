import { useEffect, useRef, useState } from 'react';
import { Modal, Tabs, Input, Slider, Button, Upload, message, Popconfirm } from 'antd';
import { websiteApi } from '../../services/web';
import type { Website } from '../../types';

const SIZE = 128; // 画布尺寸（保存为 128×128 PNG）
const SYMBOLS = ['🌐', '⭐', '⚡', '🛡️', '🔥', '🚀', '👑', '💼', '📧', '🛒', '☁️', '🎮'];
const PALETTE = ['#1677ff', '#722ed1', '#13c2c2', '#52c41a', '#fa8c16', '#eb2f96', '#f5222d', '#2f54eb', '#faad14', '#383838'];

type Mode = 'template' | 'letter' | 'upload';

interface Props {
  site: Website;
  open: boolean;
  onClose: () => void;
  onSaved: () => void;
}

// roundedRect：优先用原生 roundRect，老浏览器走手绘路径
function roundedRectPath(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  ctx.beginPath();
  if (typeof ctx.roundRect === 'function') {
    ctx.roundRect(x, y, w, h, r);
    return;
  }
  const rr = Math.min(r, w / 2, h / 2);
  ctx.moveTo(x + rr, y);
  ctx.arcTo(x + w, y, x + w, y + h, rr);
  ctx.arcTo(x + w, y + h, x, y + h, rr);
  ctx.arcTo(x, y + h, x, y, rr);
  ctx.arcTo(x, y, x + w, y, rr);
  ctx.closePath();
}

// 网站图标编辑器：模板 / 字母 / 上传三种来源，圆角·内容占比·缩放可调，
// 128×128 画布实时渲染，保存为 PNG 走既有上传端点（nginx favicon 随之生效）。
export default function IconEditor({ site, open, onClose, onSaved }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [mode, setMode] = useState<Mode>('template');
  const [symbol, setSymbol] = useState('🌐');
  const [bg, setBg] = useState('#1677ff');
  const [letter, setLetter] = useState((site.domain[0] || 'W').toUpperCase());
  const [radius, setRadius] = useState(28);
  const [scale, setScale] = useState(70);
  const [zoom, setZoom] = useState(100);
  const [img, setImg] = useState<HTMLImageElement | null>(null);
  const [saving, setSaving] = useState(false);
  const [preview, setPreview] = useState('');

  useEffect(() => {
    if (open) {
      setMode('template');
      setImg(null);
      setLetter((site.domain[0] || 'W').toUpperCase());
    }
  }, [open, site.id, site.domain]);

  // 绘制主画布并同步预览 dataURL
  useEffect(() => {
    const c = canvasRef.current;
    if (!c || !open) return;
    const ctx = c.getContext('2d');
    if (!ctx) return;
    ctx.clearRect(0, 0, SIZE, SIZE);
    const r = Math.min(radius, SIZE / 2);
    ctx.fillStyle = bg;
    roundedRectPath(ctx, 0, 0, SIZE, SIZE, r);
    ctx.fill();

    const content = SIZE * (scale / 100);
    if (mode === 'upload' && img) {
      const z = zoom / 100;
      const cover = Math.max(content / img.width, content / img.height) * z;
      const dw = img.width * cover;
      const dh = img.height * cover;
      ctx.save();
      roundedRectPath(ctx, 0, 0, SIZE, SIZE, r);
      ctx.clip();
      ctx.drawImage(img, (SIZE - dw) / 2, (SIZE - dh) / 2, dw, dh);
      ctx.restore();
    } else if (mode === 'letter') {
      ctx.fillStyle = '#ffffff';
      ctx.font = `bold ${Math.round(content * 0.72)}px system-ui, sans-serif`;
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(letter.slice(0, 2) || 'W', SIZE / 2, SIZE / 2 + SIZE * 0.02);
    } else {
      ctx.font = `${Math.round(content * 0.8)}px "Segoe UI Emoji","Apple Color Emoji","Noto Color Emoji",sans-serif`;
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(symbol, SIZE / 2, SIZE / 2 + SIZE * 0.03);
    }
    setPreview(c.toDataURL('image/png'));
  }, [open, mode, symbol, bg, letter, radius, scale, zoom, img]);

  const save = async () => {
    const c = canvasRef.current;
    if (!c) return;
    setSaving(true);
    try {
      const blob = await new Promise<Blob | null>((res) => c.toBlob(res, 'image/png'));
      if (!blob) throw new Error('生成图标失败');
      const fd = new FormData();
      fd.append('file', blob, 'icon.png');
      await websiteApi.uploadIcon(site.web_server_id, site.id, fd);
      message.success('图标已更新并应用到站点 favicon');
      onSaved();
      onClose();
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '保存失败');
    } finally {
      setSaving(false);
    }
  };

  const reset = async () => {
    try {
      await websiteApi.deleteIcon(site.web_server_id, site.id);
      message.success('已恢复默认图标');
      onSaved();
      onClose();
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '操作失败');
    }
  };

  const palette = (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
      {PALETTE.map((c) => (
        <div
          key={c}
          onClick={() => setBg(c)}
          style={{
            width: 28, height: 28, borderRadius: 6, background: c, cursor: 'pointer',
            outline: bg === c ? '2px solid #1677ff' : 'none', outlineOffset: 2,
          }}
        />
      ))}
      <input
        type="color" value={bg} title="自定义颜色"
        onChange={(e) => setBg(e.target.value)}
        style={{ width: 28, height: 28, border: 'none', background: 'none', cursor: 'pointer', padding: 0 }}
      />
    </div>
  );

  return (
    <Modal
      title={`自定义图标 - ${site.domain}`}
      open={open}
      onCancel={onClose}
      width={700}
      footer={[
        <Popconfirm key="reset" title="恢复站点默认图标？" onConfirm={reset}>
          <Button>恢复默认</Button>
        </Popconfirm>,
        <Button key="save" type="primary" loading={saving} onClick={save}>保存并应用</Button>,
      ]}
    >
      <div style={{ display: 'flex', gap: 24 }}>
        <div style={{ flexShrink: 0, textAlign: 'center' }}>
          <canvas
            ref={canvasRef} width={SIZE} height={SIZE}
            style={{ borderRadius: 12, boxShadow: '0 2px 8px rgba(0,0,0,.15)', display: 'block' }}
          />
          <div style={{ marginTop: 14, display: 'flex', alignItems: 'flex-end', gap: 12, justifyContent: 'center' }}>
            <div>
              <img src={preview} width={32} height={32} style={{ borderRadius: 8, display: 'block', margin: '0 auto' }} alt="" />
              <div style={{ fontSize: 11, opacity: 0.6, marginTop: 2 }}>列表</div>
            </div>
            <div>
              <img src={preview} width={16} height={16} style={{ display: 'block', margin: '0 auto' }} alt="" />
              <div style={{ fontSize: 11, opacity: 0.6, marginTop: 2 }}>标签页</div>
            </div>
          </div>
        </div>

        <div style={{ flex: 1, minWidth: 0 }}>
          <Tabs
            size="small"
            activeKey={mode}
            onChange={(k) => setMode(k as Mode)}
            items={[
              {
                key: 'template', label: '图标模板',
                children: (
                  <>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 14 }}>
                      {SYMBOLS.map((em) => (
                        <Button
                          key={em}
                          onClick={() => setSymbol(em)}
                          type={mode === 'template' && symbol === em ? 'primary' : 'default'}
                          style={{ fontSize: 18, width: 44, height: 40 }}
                        >
                          {em}
                        </Button>
                      ))}
                    </div>
                    {palette}
                  </>
                ),
              },
              {
                key: 'letter', label: '字母图标',
                children: (
                  <>
                    <Input
                      value={letter} maxLength={2}
                      placeholder="站点首字母，可改 1-2 个字符"
                      onChange={(e) => setLetter(e.target.value)}
                      style={{ marginBottom: 14 }}
                    />
                    {palette}
                  </>
                ),
              },
              {
                key: 'upload', label: '上传图片',
                children: (
                  <>
                    <Upload.Dragger
                      accept=".ico,.png,.svg,.jpg,.jpeg"
                      showUploadList={false} maxCount={1}
                      beforeUpload={(f) => {
                        if (f.size > 2 << 20) {
                          message.error('图片超过 2MB 限制');
                          return false;
                        }
                        const url = URL.createObjectURL(f);
                        const im = new Image();
                        im.onload = () => {
                          setImg(im);
                          setMode('upload');
                          URL.revokeObjectURL(url);
                        };
                        im.onerror = () => message.error('图片加载失败');
                        im.src = url;
                        return false;
                      }}
                    >
                      <p style={{ margin: 0 }}>点击或拖拽图片到此处</p>
                      <p style={{ fontSize: 12, opacity: 0.6, margin: '4px 0 0' }}>支持 ico / png / svg / jpg，≤2MB，自动居中裁剪填充</p>
                    </Upload.Dragger>
                    {img && <div style={{ marginTop: 8, fontSize: 12, opacity: 0.7 }}>已加载 {img.naturalWidth}×{img.naturalHeight}</div>}
                  </>
                ),
              },
            ]}
          />
          <div style={{ marginTop: 8 }}>
            <div style={{ fontSize: 13 }}>圆角 {radius}px</div>
            <Slider min={0} max={64} value={radius} onChange={setRadius} />
            <div style={{ fontSize: 13 }}>内容占比 {scale}%</div>
            <Slider min={40} max={100} value={scale} onChange={setScale} />
            {mode === 'upload' && (
              <>
                <div style={{ fontSize: 13 }}>图片缩放 {zoom}%</div>
                <Slider min={50} max={300} value={zoom} onChange={setZoom} />
              </>
            )}
          </div>
        </div>
      </div>
    </Modal>
  );
}

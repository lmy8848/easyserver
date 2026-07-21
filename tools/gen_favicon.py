#!/usr/bin/env python3
"""随机生成 EasyServer favicon 候选图标（抽象几何风格）。

用法:
    python tools/gen_favicon.py [数量]      # 默认 12 个，每次运行结果不同

输出:
    web/public/favicon-candidates/cand-NN.svg
    web/public/favicon-preview.html         # 本地浏览器打开挑

挑中后:
    cp web/public/favicon-candidates/cand-NN.svg web/public/favicon.svg
    （然后 cd web && pnpm build 重新嵌入部署）
"""
import os
import sys
import random
import colorsys


OUT_DIR = os.path.join("web", "public", "favicon-candidates")


def hsl(h, s, l):
    r, g, b = colorsys.hls_to_rgb(h / 360.0, l, s)
    return f'#{int(r * 255):02x}{int(g * 255):02x}{int(b * 255):02x}'


def luminance(hexc):
    h = hexc.lstrip('#')
    r, g, b = int(h[0:2], 16) / 255, int(h[2:4], 16) / 255, int(h[4:6], 16) / 255
    return 0.2126 * r + 0.7152 * g + 0.0722 * b


def gen_bg(rnd):
    h = rnd.random() * 360
    s = rnd.uniform(0.55, 0.85)
    l = rnd.uniform(0.42, 0.58)
    return hsl(h, s, l)


def fg_on(bg):
    return '#ffffff' if luminance(bg) < 0.55 else '#1f1f2e'


def complement(bg):
    h = bg.lstrip('#')
    r, g, b = 255 - int(h[0:2], 16), 255 - int(h[2:4], 16), 255 - int(h[4:6], 16)
    return f'#{r:02x}{g:02x}{b:02x}'


def accent(rnd, bg):
    return hsl(rnd.random() * 360, rnd.uniform(0.5, 0.85), rnd.uniform(0.55, 0.72))


# ---- 风格：每个返回 SVG 内部 body ----

def style_rings(rnd, bg, fg):
    n = rnd.randint(2, 3)
    out = []
    for i in range(n):
        r = 9 + i * 8
        sw = rnd.randint(3, 5)
        col = fg if i % 2 == 0 else complement(bg)
        out.append(f'<circle cx="32" cy="32" r="{r}" fill="none" stroke="{col}" stroke-width="{sw}"/>')
    return ''.join(out)


def style_bars(rnd, bg, fg):
    n = rnd.randint(3, 4)
    total = 44
    gap = total / (n * 1.7)
    w = gap * 0.7
    out = []
    for i in range(n):
        x = 10 + i * (gap + w * 0.3)
        h = rnd.randint(22, 44)
        y = (64 - h) / 2
        col = fg if i % 2 == 0 else accent(rnd, bg)
        out.append(f'<rect x="{x:.1f}" y="{y:.1f}" width="{w:.1f}" height="{h}" rx="2" fill="{col}"/>')
    return ''.join(out)


def style_tri(rnd, bg, fg):
    flip = rnd.choice([0, 1])
    if flip:
        return (f'<polygon points="32,10 54,46 10,46" fill="{fg}"/>'
                f'<circle cx="32" cy="35" r="6" fill="{bg}"/>')
    return (f'<polygon points="32,54 54,18 10,18" fill="{fg}"/>'
            f'<circle cx="32" cy="29" r="6" fill="{bg}"/>')


def style_letter(rnd, bg, fg):
    c = rnd.choice(['E', 'S', 'eS'])
    return (f'<text x="32" y="45" font-family="Arial,Helvetica,sans-serif" '
            f'font-size="38" font-weight="bold" text-anchor="middle" fill="{fg}">{c}</text>')


def style_dots(rnd, bg, fg):
    out = []
    for r in range(3):
        for c in range(3):
            if rnd.random() < 0.62:
                cx, cy = 16 + c * 16, 16 + r * 16
                rr = rnd.randint(3, 5)
                out.append(f'<circle cx="{cx}" cy="{cy}" r="{rr}" fill="{fg if (r + c) % 2 == 0 else accent(rnd, bg)}"/>')
    return ''.join(out)


def style_arc(rnd, bg, fg):
    a = accent(rnd, bg)
    return (f'<path d="M10 42 A22 22 0 0 1 54 42" fill="none" stroke="{fg}" stroke-width="6" stroke-linecap="round"/>'
            f'<path d="M18 42 A14 14 0 0 1 46 42" fill="none" stroke="{a}" stroke-width="5" stroke-linecap="round"/>')


def style_quad(rnd, bg, fg):
    a = accent(rnd, bg)
    return (f'<rect x="12" y="12" width="18" height="18" rx="3" fill="{fg}"/>'
            f'<rect x="34" y="12" width="18" height="18" rx="3" fill="{a}"/>'
            f'<rect x="12" y="34" width="18" height="18" rx="3" fill="{a}"/>'
            f'<rect x="34" y="34" width="18" height="18" rx="3" fill="{fg}"/>')


def style_chevron(rnd, bg, fg):
    a = accent(rnd, bg)
    return (f'<path d="M12 20 L32 12 L52 20 L32 28 Z" fill="{fg}"/>'
            f'<path d="M12 36 L32 28 L52 36 L32 44 Z" fill="{a}"/>'
            f'<path d="M12 52 L32 44 L52 52 L32 60 Z" fill="{fg}" opacity="0.85"/>')


STYLES = [style_rings, style_bars, style_tri, style_letter, style_dots,
          style_arc, style_quad, style_chevron]


def gen_one(rnd):
    bg = gen_bg(rnd)
    fg = fg_on(bg)
    style = rnd.choice(STYLES)
    body = style(rnd, bg, fg)
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" '
            f'width="64" height="64"><rect width="64" height="64" rx="14" fill="{bg}"/>{body}</svg>')


def main():
    n = int(sys.argv[1]) if len(sys.argv) > 1 else 12
    os.makedirs(OUT_DIR, exist_ok=True)
    # 清旧候选
    for old in os.listdir(OUT_DIR):
        if old.startswith('cand-') and old.endswith('.svg'):
            os.remove(os.path.join(OUT_DIR, old))

    rnd = random.Random()
    files = []
    for i in range(n):
        svg = gen_one(rnd)
        fn = f'cand-{i + 1:02d}.svg'
        with open(os.path.join(OUT_DIR, fn), 'w', encoding='utf-8') as f:
            f.write(svg)
        files.append(fn)

    cells = ''.join(
        f'<div class="cell"><div class="label">#{i + 1}</div>'
        f'<img class="big" src="favicon-candidates/{fn}"/>'
        f'<img class="small" src="favicon-candidates/{fn}"/>'
        f'<div class="fn">{fn}</div></div>'
        for i, fn in enumerate(files)
    )
    html = f'''<!doctype html><html><head><meta charset="utf-8">
<title>EasyServer favicon 候选预览</title>
<style>
body{{font-family:-apple-system,Segoe UI,sans-serif;background:#f5f5f7;margin:0;padding:24px;color:#222}}
h1{{font-size:18px;margin:0 0 4px}}
.p{{color:#666;font-size:13px;margin:0 0 20px}}
.grid{{display:grid;grid-template-columns:repeat(4,1fr);gap:14px;max-width:680px}}
.cell{{background:#fff;border-radius:12px;padding:14px;text-align:center;box-shadow:0 1px 4px rgba(0,0,0,.08)}}
img.big{{width:64px;height:64px;display:block;margin:6px auto}}
img.small{{width:16px;height:16px;border:1px solid #eee;image-rendering:pixelated}}
.label{{font-weight:700}}
.fn{{font-size:11px;color:#999;margin-top:6px}}
.note{{margin-top:20px;font-size:13px;color:#555}}
code{{background:#eee;padding:1px 5px;border-radius:4px}}
</style></head><body>
<h1>EasyServer favicon 候选</h1>
<p class="p">大图 64×64 预览，小图 16×16 模拟浏览器 tab 实际尺寸。挑一个编号告诉我。</p>
<div class="grid">{cells}</div>
<p class="note">不满意？重新运行 <code>python tools/gen_favicon.py</code> 生成新一批（每次不同）。</p>
</body></html>'''
    with open(os.path.join('web', 'public', 'favicon-preview.html'), 'w', encoding='utf-8') as f:
        f.write(html)

    print(f'生成 {n} 个候选 -> {OUT_DIR}')
    print('预览: 浏览器打开 web/public/favicon-preview.html （或 dev server 访问 /favicon-preview.html）')


if __name__ == '__main__':
    main()

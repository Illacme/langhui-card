#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
生成「常用开源素材库」（输出到 ./library/，由 assets.go 用 go:embed 内嵌进后端二进制）。

素材库目标：PC 后台所有需要图片的位置（分类图标 / 服务图标 / 项目封面 / 相册 /
公司简介封面 / 公司 LOGO）都能直接选内置素材，免去四处找图、按尺寸处理图的过程。

三类素材：
  icons/   Lucide（ISC 许可）线形图标，rsvg-convert 光栅化为白色图形后合成到
           渐变圆角方块上——与小程序 .cat-placeholder（120rpx 圆角蓝砖 + 白字）
           的既有观感一致，客户已确认的设计语言。
  covers/  程序生成的通用封面（渐变 + 几何 motif，不含文字，任何企业/项目可复用）。
  logos/   程序生成的通用 LOGO 标识（渐变圆角方块 + 白色几何标记）。
  avatars/ 员工暂无照片时的默认头像（渐变圆角方块 + 白色人形剪影）。

依赖：Pillow（PIL）+ rsvg-convert（librsvg）。Lucide SVG 首次运行从 jsdelivr 下载，
留存到 library-src/icons/（附 LICENSE，ISC 许可），之后离线也可再生成。

用法：
  python3 gen_library.py
"""
import json
import os
import random
import subprocess
import tempfile

from PIL import Image, ImageDraw

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, "library")
SRC = os.path.join(HERE, "library-src", "icons")
LUCIDE_VERSION = "0.469.0"
LUCIDE_BASE = "https://unpkg.com/lucide-static@%s/icons" % LUCIDE_VERSION

# ── 配色 ──────────────────────────────────────────────────────────────
# 分组配色：分类=品牌蓝（与已确认的设计稿一致）、服务=靛蓝、通用=青
GROUP_COLORS = {
    "cat": ((26, 140, 255), (14, 111, 224)),    # #1A8CFF → #0E6FE0
    "svc": ((91, 108, 255), (61, 79, 224)),     # #5B6CFF → #3D4FE0
    "misc": ((20, 184, 166), (13, 148, 136)),   # #14B8A6 → #0D9488
}

# (lucide 名, 中文名, 分组, 搜索标签)
ICONS = [
    # ── 业绩分类 ──
    ("house", "居住住宅", "cat", ["住宅", "楼宇", "home"]),
    ("landmark", "政务办公", "cat", ["政务", "government", "机关"]),
    ("hospital", "医院", "cat", ["医疗", "hospital", "卫生"]),
    ("school", "学校", "cat", ["教育", "school", "校园"]),
    ("shopping-bag", "商业综合体", "cat", ["商业", "购物", "商场"]),
    ("train-front", "地铁站点", "cat", ["地铁", "交通", "轨道"]),
    ("factory", "生产制造", "cat", ["工厂", "制造", "车间"]),
    ("warehouse", "仓储物流", "cat", ["仓储", "物流", "仓库"]),
    ("building-2", "楼宇建筑", "cat", ["楼宇", "建筑", "building"]),
    ("briefcase", "商务办公", "cat", ["商务", "办公", "写字楼"]),
    ("graduation-cap", "教育培训", "cat", ["教育", "培训", "高校"]),
    ("hotel", "酒店住宿", "cat", ["酒店", "住宿", "宾馆"]),
    ("utensils", "餐饮美食", "cat", ["餐饮", "美食", "餐厅"]),
    ("bus", "公共交通", "cat", ["公交", "交通", "客运"]),
    ("zap", "能源电力", "cat", ["能源", "电力", "电气"]),
    ("leaf", "绿色环保", "cat", ["环保", "绿色", "节能"]),
    ("shield-check", "安防保障", "cat", ["安防", "保障", "安全"]),
    ("truck", "物流配送", "cat", ["物流", "配送", "运输"]),
    # ── 业务服务 ──
    ("arrow-up-down", "电梯", "svc", ["电梯", "垂直交通", "elevator"]),
    ("activity", "运行平稳", "svc", ["运行", "平稳", "activity"]),
    ("wrench", "维修服务", "svc", ["维修", "检修", "wrench"]),
    ("cog", "维保服务", "svc", ["维保", "保养", "运维"]),
    ("clipboard-check", "检测验收", "svc", ["检测", "验收", "质检"]),
    ("ruler", "测量定制", "svc", ["测量", "定制", "勘察"]),
    ("hard-hat", "安装施工", "svc", ["安装", "施工", "工程"]),
    ("phone", "咨询预约", "svc", ["咨询", "预约", "电话"]),
    ("clock", "快速响应", "svc", ["响应", "时效", "时钟"]),
    ("headset", "售后客服", "svc", ["售后", "客服", "支持"]),
    ("package", "交付配送", "svc", ["交付", "配送", "发货"]),
    ("handshake", "合作共赢", "svc", ["合作", "共赢", "签约"]),
    ("star", "品质服务", "svc", ["品质", "优质", "星级"]),
    ("life-buoy", "全程保障", "svc", ["保障", "救援", "支持"]),
    ("timer", "限时响应", "svc", ["限时", "计时", "加急"]),
    ("badge-check", "认证合格", "svc", ["认证", "合格", "资质"]),
    # ── 通用 ──
    ("image", "图片", "misc", ["图片", "照片", "image"]),
    ("users", "人群团队", "misc", ["团队", "人群", "用户"]),
    ("award", "荣誉奖项", "misc", ["荣誉", "奖项", "奖杯"]),
    ("gem", "精品案例", "misc", ["精品", "案例", "宝石"]),
    ("lightbulb", "方案创意", "misc", ["创意", "方案", "点子"]),
    ("map-pin", "位置服务", "misc", ["位置", "地图", "定位"]),
]

# 封面：渐变 + 几何 motif，无文字
COVERS = [
    ("cover-skyline", "天际线", ((11, 61, 145), (21, 101, 192))),
    ("cover-dots", "点阵", ((67, 56, 202), (99, 102, 241))),
    ("cover-beams", "光束", ((15, 118, 110), (20, 184, 166))),
    ("cover-arcs", "弧光", ((109, 40, 217), (139, 92, 246))),
    ("cover-blocks", "矩阵", ((51, 65, 85), (100, 116, 139))),
    ("cover-shield", "守护", ((21, 128, 61), (34, 197, 94))),
    ("cover-motion", "疾驰", ((234, 88, 12), (249, 115, 22))),
    ("cover-stars", "星网", ((15, 23, 42), (30, 58, 138))),
    ("cover-waves", "波浪", ((14, 116, 144), (6, 182, 212))),
    ("cover-hex", "蜂巢", ((190, 18, 60), (244, 63, 94))),
]

# LOGO：渐变圆角方块 + 白色几何标记
LOGOS = [
    ("logo-building", "建筑", ((26, 140, 255), (14, 111, 224))),
    ("logo-bolt", "闪电", ((245, 158, 11), (217, 119, 6))),
    ("logo-gear", "齿轮", ((71, 85, 105), (30, 41, 59))),
    ("logo-shield", "盾牌", ((21, 128, 61), (22, 101, 52))),
    ("logo-hex", "六边", ((109, 40, 217), (91, 33, 182))),
    ("logo-arrow", "上升", ((20, 184, 166), (15, 118, 110))),
    ("logo-tower", "塔楼", ((91, 108, 255), (61, 79, 224))),
    ("logo-orbit", "环轨", ((244, 63, 94), (190, 18, 60))),
]

COVER_W, COVER_H = 960, 504
THUMB_W, THUMB_H = 480, 252

# 头像：员工暂无照片时的默认头像（渐变圆角方块 + 白色人形剪影）
AVATARS = [
    ("avatar-1", "默认头像 · 蓝", ((26, 140, 255), (14, 111, 224))),
    ("avatar-2", "默认头像 · 靛", ((91, 108, 255), (61, 79, 224))),
    ("avatar-3", "默认头像 · 青", ((20, 184, 166), (13, 148, 136))),
    ("avatar-4", "默认头像 · 紫", ((139, 92, 246), (109, 40, 217))),
    ("avatar-5", "默认头像 · 绿", ((34, 197, 94), (21, 128, 61))),
    ("avatar-6", "默认头像 · 橙", ((249, 115, 22), (234, 88, 12))),
    ("avatar-7", "默认头像 · 玫", ((244, 63, 94), (190, 18, 60))),
    ("avatar-8", "默认头像 · 灰", ((100, 116, 139), (51, 65, 85))),
]


# ── 基础绘制 ──────────────────────────────────────────────────────────
def vgrad(size, top, bottom):
    """竖直渐变 RGB 图。"""
    w, h = size
    img = Image.new("RGB", (w, h), top)
    d = ImageDraw.Draw(img)
    for y in range(h):
        t = y / max(1, h - 1)
        c = tuple(int(top[i] + (bottom[i] - top[i]) * t) for i in range(3))
        d.line([(0, y), (w, y)], fill=c)
    return img


def rounded_tile(size, top, bottom, radius_ratio=0.22, ss=4):
    """圆角渐变方块（RGBA，圆角外透明）；ss 倍超采样抗锯齿。"""
    S = size * ss
    img = vgrad((S, S), top, bottom).convert("RGBA")
    mask = Image.new("L", (S, S), 0)
    ImageDraw.Draw(mask).rounded_rectangle(
        [0, 0, S - 1, S - 1], radius=int(S * radius_ratio), fill=255
    )
    img.putalpha(mask)
    return img.resize((size, size), Image.LANCZOS)


def http_get(url, dst=None, retries=4, valid=None):
    """经 curl 下载（代理环境下比 urllib 稳，带重试）；dst 为 None 时返回字节。
    valid(bytes) -> bool：内容校验（防代理截断出半截文件），不合法则重试。"""
    last_err = "unknown"
    for attempt in range(retries):
        cmd = ["curl", "-sSL", "--max-time", "30", "--retry", "2"]
        if dst:
            cmd += ["-o", dst]
        cmd.append(url)
        r = subprocess.run(cmd, capture_output=True)
        if r.returncode == 0:
            if dst:
                with open(dst, "rb") as f:
                    data = f.read()
            else:
                data = r.stdout
            if valid is None or valid(data):
                return data if not dst else True
            last_err = "invalid content (%d bytes)" % len(data)
            if dst and os.path.exists(dst):
                os.unlink(dst)
        else:
            last_err = "curl exit %d" % r.returncode
        print("  download retry %d (%s): %s" % (attempt + 1, last_err, url))
    raise RuntimeError("download failed: " + url + " (" + last_err + ")")


def is_svg(data):
    """SVG 内容校验：非空且以 </svg> 收尾（防代理截断出半截文件）。"""
    return len(data) > 100 and data.rstrip().endswith(b"</svg>")


def fetch_lucide(name):
    """下载 Lucide SVG 到 library-src/icons/（已存在则跳过）。"""
    os.makedirs(SRC, exist_ok=True)
    dst = os.path.join(SRC, name + ".svg")
    if os.path.exists(dst):
        return dst
    data = http_get("%s/%s.svg" % (LUCIDE_BASE, name), valid=is_svg)
    with open(dst, "wb") as f:
        f.write(data)
    print("  fetched %s (%d bytes)" % (name, len(data)))
    return dst


def render_glyph(svg_path, size, stroke_scale=0.75):
    """Lucide SVG → 指定尺寸的白色图形 PNG（透明底，rsvg-convert 光栅化）。"""
    src = open(svg_path, encoding="utf-8").read()
    src = src.replace('stroke="currentColor"', 'stroke="#FFFFFF"')
    src = src.replace('stroke-width="2"', 'stroke-width="%.2f"' % (2 * stroke_scale))
    with tempfile.TemporaryDirectory() as td:
        svg = os.path.join(td, "g.svg")
        png = os.path.join(td, "g.png")
        with open(svg, "w", encoding="utf-8") as f:
            f.write(src)
        subprocess.run(
            ["rsvg-convert", "-w", str(size), "-h", str(size), "-o", png, svg],
            check=True, capture_output=True,
        )
        return Image.open(png).convert("RGBA")


def make_icon(item_id, name, lucide, group, tags):
    S = 256
    tile = rounded_tile(S, *GROUP_COLORS[group])
    glyph = render_glyph(fetch_lucide(lucide), int(S * 0.62))
    tile.alpha_composite(glyph, ((S - glyph.width) // 2, (S - glyph.height) // 2))
    path = os.path.join(OUT, "icons", item_id + ".png")
    tile.save(path, "PNG")
    return {"id": item_id, "name": name, "category": "icon",
            "file": "icons/%s.png" % item_id, "thumb": "icons/%s.png" % item_id,
            "width": S, "height": S, "tags": tags}


# ── 封面 motif ────────────────────────────────────────────────────────
def overlay(base):
    """在渐变底上叠一层全幅 RGBA 画布（统一用半透明白色作图形色）。"""
    ov = Image.new("RGBA", base.size, (0, 0, 0, 0))
    return ov, ImageDraw.Draw(ov)


def motif_skyline(img):
    ov, d = overlay(img)
    w, h = img.size
    random.seed(11)
    x = 40
    while x < w - 60:
        bw = random.choice([70, 90, 110, 130])
        bh = random.randint(int(h * 0.35), int(h * 0.72))
        top = h - bh
        d.rectangle([x, top, x + bw, h], fill=(255, 255, 255, 46))
        for wy in range(top + 14, h - 12, 22):
            for wx in range(x + 10, x + bw - 8, 20):
                if random.random() < 0.55:
                    d.rectangle([wx, wy, wx + 9, wy + 11], fill=(255, 255, 255, 120))
        x += bw + random.randint(14, 26)
    img.alpha_composite(ov)


def motif_dots(img):
    ov, d = overlay(img)
    w, h = img.size
    for gy in range(30, h, 44):
        for gx in range(30, w, 44):
            d.ellipse([gx - 3, gy - 3, gx + 3, gy + 3], fill=(255, 255, 255, 70))
    d.ellipse([w * 0.62, h * 0.12, w * 0.62 + 190, h * 0.12 + 190],
              outline=(255, 255, 255, 150), width=10)
    img.alpha_composite(ov)


def motif_beams(img):
    ov, d = overlay(img)
    w, h = img.size
    for i, alpha in enumerate((40, 70, 110, 160)):
        x0 = -200 + i * 190
        d.polygon([(x0, h), (x0 + 90, h), (x0 + 90 + h, 0), (x0 + h, 0)],
                  fill=(255, 255, 255, alpha))
    img.alpha_composite(ov)


def motif_arcs(img):
    ov, d = overlay(img)
    w, h = img.size
    cx, cy = w * 0.78, h * 1.05
    for i, r in enumerate(range(120, 620, 90)):
        d.arc([cx - r, cy - r, cx + r, cy + r], 180, 360,
              fill=(255, 255, 255, 60 + i * 22), width=12)
    img.alpha_composite(ov)


def motif_blocks(img):
    ov, d = overlay(img)
    w, h = img.size
    random.seed(23)
    for row in range(4):
        for col in range(9):
            bw = random.randint(50, 110)
            bh = random.randint(30, 90)
            x = 30 + col * 105 + random.randint(-8, 8)
            y = 30 + row * 120 + random.randint(-8, 8)
            d.rounded_rectangle([x, y, x + bw, y + bh], radius=10,
                                fill=(255, 255, 255, random.randint(28, 84)))
    img.alpha_composite(ov)


def motif_shield(img):
    ov, d = overlay(img)
    w, h = img.size
    cx, cy, s = w * 0.5, h * 0.52, 130
    pts = [(cx, cy - s), (cx + s * 0.85, cy - s * 0.62), (cx + s * 0.85, cy + s * 0.18),
           (cx, cy + s), (cx - s * 0.85, cy + s * 0.18), (cx - s * 0.85, cy - s * 0.62)]
    d.line(pts + [pts[0]], fill=(255, 255, 255, 200), width=12, joint="curve")
    d.line([(cx - 52, cy - 4), (cx - 12, cy + 40), (cx + 60, cy - 44)],
           fill=(255, 255, 255, 220), width=16, joint="curve")
    img.alpha_composite(ov)


def motif_motion(img):
    ov, d = overlay(img)
    w, h = img.size
    random.seed(31)
    for i in range(16):
        y = 26 + i * 30
        ln = random.randint(int(w * 0.25), int(w * 0.85))
        d.rounded_rectangle([40, y, 40 + ln, y + 12], radius=6,
                            fill=(255, 255, 255, random.randint(36, 120)))
    img.alpha_composite(ov)


def motif_stars(img):
    ov, d = overlay(img)
    w, h = img.size
    random.seed(41)
    for _ in range(90):
        x, y = random.randint(0, w), random.randint(0, int(h * 0.7))
        r = random.choice([2, 2, 3, 4])
        d.ellipse([x - r, y - r, x + r, y + r], fill=(255, 255, 255, random.randint(90, 200)))
    # 透视网格
    horizon = int(h * 0.66)
    for i in range(-8, 9):
        d.line([(w / 2 + i * 60, horizon), (w / 2 + i * 260, h)],
               fill=(255, 255, 255, 60), width=2)
    for j in range(1, 7):
        y = horizon + (h - horizon) * (j / 6) ** 1.6
        d.line([(0, y), (w, y)], fill=(255, 255, 255, 50), width=2)
    img.alpha_composite(ov)


def motif_waves(img):
    import math
    ov, d = overlay(img)
    w, h = img.size
    for k, alpha in enumerate((50, 80, 120)):
        base = h * (0.55 + k * 0.14)
        amp = 26 - k * 5
        pts = [(x, base + amp * math.sin((x / w) * math.pi * 2 + k * 1.2))
               for x in range(0, w + 8, 8)]
        d.polygon(pts + [(w, h), (0, h)], fill=(255, 255, 255, alpha))
    img.alpha_composite(ov)


def motif_hex(img):
    import math
    ov, d = overlay(img)
    w, h = img.size
    r = 46
    for row in range(-1, 8):
        for col in range(-1, 13):
            cx = col * r * 1.5
            cy = row * r * math.sqrt(3) + (col % 2) * r * math.sqrt(3) / 2
            pts = [(cx + r * math.cos(math.radians(60 * i)),
                    cy + r * math.sin(math.radians(60 * i))) for i in range(6)]
            d.line(pts + [pts[0]], fill=(255, 255, 255, 70), width=3, joint="curve")
    img.alpha_composite(ov)


COVER_MOTIFS = {
    "cover-skyline": motif_skyline, "cover-dots": motif_dots, "cover-beams": motif_beams,
    "cover-arcs": motif_arcs, "cover-blocks": motif_blocks, "cover-shield": motif_shield,
    "cover-motion": motif_motion, "cover-stars": motif_stars, "cover-waves": motif_waves,
    "cover-hex": motif_hex,
}


def make_cover(item_id, name, colors):
    img = vgrad((COVER_W, COVER_H), *colors).convert("RGBA")
    COVER_MOTIFS[item_id](img)
    path = os.path.join(OUT, "covers", item_id + ".jpg")
    img.convert("RGB").save(path, "JPEG", quality=87)
    # 预览缩略图（选择器里加载，避免原图流量）
    thumb = img.convert("RGB").resize((THUMB_W, THUMB_H), Image.LANCZOS)
    tpath = os.path.join(OUT, "thumbs", "covers", item_id + ".jpg")
    thumb.save(tpath, "JPEG", quality=80)
    return {"id": item_id, "name": name, "category": "cover",
            "file": "covers/%s.jpg" % item_id, "thumb": "thumbs/covers/%s.jpg" % item_id,
            "width": COVER_W, "height": COVER_H, "tags": [name, "封面", "背景"]}


# ── LOGO 标记 ─────────────────────────────────────────────────────────
def _mark_layer(size):
    return Image.new("RGBA", (size, size), (0, 0, 0, 0))


def mark_building(size):
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    w = int(size * 0.05)
    d.rounded_rectangle([size * 0.26, size * 0.18, size * 0.74, size * 0.82],
                        radius=int(size * 0.04), outline=(255, 255, 255, 255), width=w)
    d.line([(size * 0.20, size * 0.82), (size * 0.80, size * 0.82)],
           fill=(255, 255, 255, 255), width=w)
    for r in range(2):
        for c in range(3):
            x = size * (0.34 + c * 0.14)
            y = size * (0.28 + r * 0.20)
            d.rectangle([x, y, x + size * 0.08, y + size * 0.10],
                        outline=(255, 255, 255, 230), width=int(w * 0.7))
    return lay


def mark_bolt(size):
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    d.polygon([(size * 0.56, size * 0.14), (size * 0.30, size * 0.54),
               (size * 0.46, size * 0.54), (size * 0.40, size * 0.86),
               (size * 0.70, size * 0.44), (size * 0.53, size * 0.44)],
              fill=(255, 255, 255, 255))
    return lay


def mark_gear(size):
    import math
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    cx = cy = size / 2
    r_out, r_in = size * 0.30, size * 0.21
    for i in range(8):
        a = math.radians(i * 45)
        tx, ty = cx + r_out * 0.92 * math.cos(a), cy + r_out * 0.92 * math.sin(a)
        tooth = Image.new("RGBA", (int(size * 0.12), int(size * 0.12)), (0, 0, 0, 0))
        ImageDraw.Draw(tooth).rounded_rectangle(
            [0, 0, size * 0.12 - 1, size * 0.12 - 1], radius=int(size * 0.02),
            fill=(255, 255, 255, 255))
        tooth = tooth.rotate(-i * 45, expand=True, resample=Image.BICUBIC)
        lay.alpha_composite(tooth, (int(tx - tooth.width / 2), int(ty - tooth.height / 2)))
    d.ellipse([cx - r_out, cy - r_out, cx + r_out, cy + r_out],
              outline=(255, 255, 255, 255), width=int(size * 0.05))
    d.ellipse([cx - r_in, cy - r_in, cx + r_in, cy + r_in],
              outline=(255, 255, 255, 255), width=int(size * 0.05))
    return lay


def mark_shield(size):
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    cx, cy, s = size / 2, size * 0.48, size * 0.30
    pts = [(cx, cy - s), (cx + s * 0.85, cy - s * 0.62), (cx + s * 0.85, cy + s * 0.18),
           (cx, cy + s), (cx - s * 0.85, cy + s * 0.18), (cx - s * 0.85, cy - s * 0.62)]
    d.line(pts + [pts[0]], fill=(255, 255, 255, 255), width=int(size * 0.045), joint="curve")
    d.line([(cx - s * 0.38, cy - s * 0.02), (cx - s * 0.08, cy + s * 0.28),
            (cx + s * 0.44, cy - s * 0.34)],
           fill=(255, 255, 255, 255), width=int(size * 0.05), joint="curve")
    return lay


def mark_hex(size):
    import math
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    cx = cy = size / 2
    for r, wd in ((size * 0.30, int(size * 0.05)), (size * 0.17, int(size * 0.045))):
        pts = [(cx + r * math.cos(math.radians(60 * i + 30)),
                cy + r * math.sin(math.radians(60 * i + 30))) for i in range(6)]
        d.line(pts + [pts[0]], fill=(255, 255, 255, 255), width=wd, joint="curve")
    return lay


def mark_arrow(size):
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    cx, cy, r = size / 2, size / 2, size * 0.30
    d.ellipse([cx - r, cy - r, cx + r, cy + r],
              outline=(255, 255, 255, 255), width=int(size * 0.05))
    d.polygon([(cx, cy - r * 0.62), (cx + r * 0.42, cy + r * 0.02),
               (cx - r * 0.42, cy + r * 0.02)], fill=(255, 255, 255, 255))
    d.rectangle([cx - r * 0.14, cy + r * 0.02, cx + r * 0.14, cy + r * 0.58],
                fill=(255, 255, 255, 255))
    return lay


def mark_tower(size):
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    d.polygon([(size * 0.40, size * 0.16), (size * 0.60, size * 0.16),
               (size * 0.68, size * 0.82), (size * 0.32, size * 0.82)],
              outline=(255, 255, 255, 255), width=int(size * 0.045))
    for i in range(3):
        y = size * (0.34 + i * 0.16)
        d.line([(size * (0.40 - i * 0.015), y), (size * (0.60 + i * 0.015), y)],
               fill=(255, 255, 255, 230), width=int(size * 0.035))
    d.line([(size * 0.50, size * 0.16), (size * 0.50, size * 0.06)],
           fill=(255, 255, 255, 255), width=int(size * 0.035))
    return lay


def mark_orbit(size):
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    cx, cy = size / 2, size / 2
    d.ellipse([cx - size * 0.13, cy - size * 0.13, cx + size * 0.13, cy + size * 0.13],
              fill=(255, 255, 255, 255))
    d.ellipse([cx - size * 0.32, cy - size * 0.18, cx + size * 0.32, cy + size * 0.18],
              outline=(255, 255, 255, 230), width=int(size * 0.04))
    d.ellipse([cx + size * 0.26, cy - size * 0.07, cx + size * 0.36, cy + size * 0.03],
              fill=(255, 255, 255, 255))
    return lay


def mark_person(size):
    """白色人形剪影（头 + 肩），用于默认头像。"""
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    d.ellipse([size * 0.36, size * 0.14, size * 0.64, size * 0.42], fill=(255, 255, 255, 255))
    d.pieslice([size * 0.18, size * 0.46, size * 0.82, size * 1.10], 180, 360,
               fill=(255, 255, 255, 255))
    return lay


def make_avatar(item_id, name, colors):
    S = 256
    tile = rounded_tile(S, *colors, radius_ratio=0.22)
    tile.alpha_composite(mark_person(S), (0, 0))
    tile.save(os.path.join(OUT, "avatars", item_id + ".png"), "PNG")
    return {"id": item_id, "name": name, "category": "avatar",
            "file": "avatars/%s.png" % item_id, "thumb": "avatars/%s.png" % item_id,
            "width": S, "height": S, "tags": [name, "头像", "默认"]}


LOGO_MARKS = {
    "logo-building": mark_building, "logo-bolt": mark_bolt, "logo-gear": mark_gear,
    "logo-shield": mark_shield, "logo-hex": mark_hex, "logo-arrow": mark_arrow,
    "logo-tower": mark_tower, "logo-orbit": mark_orbit,
}


def make_logo(item_id, name, colors):
    S = 512
    tile = rounded_tile(S, *colors, radius_ratio=0.22)
    mark = LOGO_MARKS[item_id](S)
    tile.alpha_composite(mark, (0, 0))
    path = os.path.join(OUT, "logos", item_id + ".png")
    tile.save(path, "PNG")
    return {"id": item_id, "name": name, "category": "logo",
            "file": "logos/%s.png" % item_id, "thumb": "logos/%s.png" % item_id,
            "width": S, "height": S, "tags": [name, "LOGO", "标识"]}


def fetch_lucide_license():
    os.makedirs(os.path.dirname(SRC), exist_ok=True)
    dst = os.path.join(HERE, "library-src", "LICENSE-Lucide.txt")
    if os.path.exists(dst):
        return
    url = "https://unpkg.com/lucide-static@%s/LICENSE" % LUCIDE_VERSION
    http_get(url, dst)
    print("  fetched Lucide LICENSE")


CREDITS = """# 常用开源素材库 · 许可与来源

本目录由 `gen_library.py` 生成，内嵌进后端二进制（见 `assets.go`）。

- **图标（icons/）**：取自 [Lucide](https://lucide.dev) v%s（ISC License），
  经 rsvg-convert 光栅化并合成到渐变圆角方块。ISC 许可全文见
  `../library-src/LICENSE-Lucide.txt`。
- **封面（covers/）、LOGO（logos/）与头像（avatars/）**：由生成脚本程序绘制
  （渐变 + 几何图形/人形剪影），无第三方素材，可自由使用。

重新生成：`python3 gen_library.py`
（依赖 Pillow 与 rsvg-convert；Lucide SVG 已留存 library-src/icons/，离线可再生成）。
"""


def main():
    for sub in ("icons", "covers", "logos", "avatars", "thumbs/covers"):
        os.makedirs(os.path.join(OUT, sub), exist_ok=True)
    fetch_lucide_license()

    items = []
    print("icons: %d" % len(ICONS))
    for lucide, name, group, tags in ICONS:
        items.append(make_icon("icon-" + lucide, name, lucide, group, tags))
    print("covers: %d" % len(COVERS))
    for cid, name, colors in COVERS:
        items.append(make_cover(cid, name, colors))
    print("logos: %d" % len(LOGOS))
    for lid, name, colors in LOGOS:
        items.append(make_logo(lid, name, colors))
    print("avatars: %d" % len(AVATARS))
    for aid, name, colors in AVATARS:
        items.append(make_avatar(aid, name, colors))

    manifest = {
        "version": 1,
        "categories": [
            {"key": "icon", "name": "图标"},
            {"key": "cover", "name": "封面"},
            {"key": "logo", "name": "LOGO"},
            {"key": "avatar", "name": "头像"},
        ],
        "items": items,
    }
    with open(os.path.join(OUT, "manifest.json"), "w", encoding="utf-8") as f:
        json.dump(manifest, f, ensure_ascii=False, indent=1)
    with open(os.path.join(OUT, "CREDITS.md"), "w", encoding="utf-8") as f:
        f.write(CREDITS % LUCIDE_VERSION)

    print("done: %d items -> %s" % (len(items), OUT))


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
# 生成朗汇机电小程序设计还原用的蓝色主题 PNG 占位素材。
# 这些素材仅用于本地联调/演示占位；正式上线请在 PC 后台上传真实图片替换。
import os
from PIL import Image, ImageDraw, ImageFont

OUT = os.path.join(os.path.dirname(__file__), "seedassets")

PRIMARY = (26, 140, 255)   # #1A8CFF 科技亮蓝
DARK = (14, 111, 224)      # #0E6FE0
LIGHT = (232, 243, 255)    # #E8F3FF
WHITE = (255, 255, 255)

FONT_PATH = "/System/Library/Fonts/STHeiti Medium.ttc"


def font(size, idx=0):
    try:
        return ImageFont.truetype(FONT_PATH, size, index=idx)
    except Exception:
        return ImageFont.load_default()


def gradient(w, h, top, bottom):
    img = Image.new("RGB", (w, h), top)
    d = ImageDraw.Draw(img)
    for y in range(h):
        t = y / max(1, h - 1)
        c = tuple(int(top[i] + (bottom[i] - top[i]) * t) for i in range(3))
        d.line([(0, y), (w, y)], fill=c)
    return img


def rr(d, box, r, fill):
    d.rounded_rectangle(box, radius=r, fill=fill)


def center_text(d, box, text, fnt, fill):
    d.text((box[0] + (box[2] - box[0]) / 2, box[1] + (box[3] - box[1]) / 2),
           text, font=fnt, fill=fill, anchor="mm")


def save(img, name):
    p = os.path.join(OUT, name)
    img.save(p, "PNG")
    print("wrote", p)


def make_logo():
    img = gradient(200, 200, PRIMARY, DARK)
    d = ImageDraw.Draw(img)
    d.ellipse([58, 40, 142, 124], fill=WHITE)
    center_text(d, [58, 40, 142, 124], "朗汇", font(40), PRIMARY)
    center_text(d, [0, 130, 200, 180], "机电设备", font(24), WHITE)
    save(img, "logo.png")


def make_avatar(char):
    img = gradient(200, 200, LIGHT, (210, 232, 255))
    d = ImageDraw.Draw(img)
    d.ellipse([20, 20, 180, 180], fill=PRIMARY)
    d.ellipse([34, 34, 166, 166], outline=WHITE, width=4)
    center_text(d, [20, 20, 180, 180], char, font(96), WHITE)
    save(img, "avatar-gaolixin.png")


def make_icon(name, label):
    img = gradient(160, 160, PRIMARY, DARK)
    d = ImageDraw.Draw(img)
    rr(d, [10, 10, 150, 150], 28, None)
    d.rounded_rectangle([10, 10, 150, 150], radius=28, outline=WHITE, width=3)
    center_text(d, [10, 40, 150, 140], label, font(46), WHITE)
    save(img, name)


def make_cover(name, title, subtitle=""):
    img = gradient(640, 360, PRIMARY, DARK)
    d = ImageDraw.Draw(img)
    # 简单楼宇剪影
    d.rectangle([60, 150, 200, 320], fill=(255, 255, 255, 60))
    d.rectangle([230, 90, 360, 320], fill=(255, 255, 255, 80))
    d.rectangle([390, 180, 540, 320], fill=(255, 255, 255, 60))
    for x in range(70, 195, 26):
        d.rectangle([x, 165, x + 12, 305], fill=(255, 255, 255, 120))
    center_text(d, [0, 250, 640, 320], title, font(44), WHITE)
    if subtitle:
        center_text(d, [0, 320, 640, 360], subtitle, font(24), (235, 245, 255))
    save(img, name)


def make_album(name, label):
    img = gradient(400, 300, (110, 170, 255), DARK)
    d = ImageDraw.Draw(img)
    center_text(d, [0, 0, 400, 300], label, font(34), WHITE)
    save(img, name)


def main():
    os.makedirs(OUT, exist_ok=True)
    make_logo()
    make_avatar("高")
    cats = [
        ("cat-1.png", "住宅"), ("cat-2.png", "政务"), ("cat-3.png", "医校"),
        ("cat-4.png", "商酒"), ("cat-5.png", "公交"), ("cat-6.png", "制造"),
        ("cat-7.png", "私宅"),
    ]
    for n, l in cats:
        make_icon(n, l)
    make_cover("cover-company.png", "朗汇机电 · 企业宣传")
    make_cover("cover-daxing.png", "大兴临空服务中心")
    make_cover("cover-hospital.png", "某医院电梯项目")
    svcs = [
        ("svc-1.png", "销售"), ("svc-2.png", "安装"),
        ("svc-3.png", "维保"), ("svc-4.png", "改造"),
    ]
    for n, l in svcs:
        make_icon(n, l)
    make_album("album-1.png", "项目实景 ①")
    make_album("album-2.png", "项目实景 ②")
    print("done")


if __name__ == "__main__":
    main()

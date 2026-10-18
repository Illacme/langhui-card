#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
生成「常用开源素材库」（输出到 ./library/，由 assets.go 用 go:embed 内嵌进后端二进制）。

素材库目标：PC 后台所有需要图片的位置（分类图标 / 服务图标 / 项目封面 / 相册 /
公司简介封面 / 公司 LOGO / 员工头像 / 小程序首页背景）都能直接选内置素材，
免去四处找图、按尺寸处理图的过程。

四类素材：
  icons/   Lucide（ISC 许可）线形图标，rsvg-convert 光栅化为白色图形后合成到
           渐变圆角方块上——与小程序 .cat-placeholder（120rpx 圆角蓝砖 + 白字）
           的既有观感一致，客户已确认的设计语言。
  covers/  程序生成的通用封面（渐变 + 几何 motif，不含文字，任何企业/项目可复用）。
  logos/   程序生成的通用 LOGO 标识（渐变圆角方块 + 白色几何标记）。
  avatars/ 员工暂无照片时的默认头像（渐变圆角方块 + 白色人形剪影）。
  backgrounds/ 背景图（750×1334 竖幅：深色高端商务渐变 + 几何 motif，
           白卡片浮层其上，营造层次感；亦含浅色系供浅色品牌调性选用。

素材按「构图 motif + 配色」解耦：同一 motif 换配色即可派生新款式，
因此扩充时往往只需在 COVERS / LOGOS / BACKGROUNDS 里加一行。

依赖：Pillow（PIL）+ rsvg-convert（librsvg）。Lucide SVG 首次运行从 jsdelivr 下载，
留存到 library-src/icons/（附 LICENSE，ISC 许可），之后离线也可再生成。

用法：
  python3 gen_library.py
"""
import json
import math
import os
import random
import subprocess
import tempfile

from PIL import Image, ImageDraw, ImageFilter

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
    ("plane", "航空机场", "cat", ["航空", "机场", "飞机"]),
    ("ship", "港口航运", "cat", ["港口", "航运", "船舶"]),
    ("store", "商铺零售", "cat", ["商铺", "零售", "门店"]),
    ("library", "文化场馆", "cat", ["文化", "场馆", "图书"]),
    ("flask-conical", "科研实验", "cat", ["科研", "实验", "检测"]),
    ("microscope", "生物医疗", "cat", ["生物", "医疗", "显微"]),
    ("atom", "高新科技", "cat", ["科技", "高新", "原子"]),
    ("dumbbell", "文体健身", "cat", ["健身", "文体", "运动"]),
    ("trees", "园林绿化", "cat", ["园林", "绿化", "景观"]),
    ("wind", "风力新能源", "cat", ["风电", "新能源", "风力"]),
    ("sun", "光伏太阳能", "cat", ["光伏", "太阳能", "发电"]),
    ("droplet", "水务水利", "cat", ["水务", "水利", "给排水"]),
    ("flame", "热力能源", "cat", ["热力", "能源", "燃烧"]),
    ("recycle", "再生资源", "cat", ["再生", "资源", "循环"]),
    ("car", "汽车交通", "cat", ["汽车", "交通", "车行"]),
    ("bike", "慢行系统", "cat", ["慢行", "骑行", "绿道"]),
    ("anchor", "港航码头", "cat", ["港航", "码头", "锚地"]),
    ("church", "文化宗教", "cat", ["文化", "宗教", "礼堂"]),
    ("banknote", "金融服务", "cat", ["金融", "银行", "货币"]),
    ("piggy-bank", "投资理财", "cat", ["投资", "理财", "财富"]),
    ("hammer", "工程施工", "cat", ["施工", "工程", "建造"]),
    ("mountain-snow", "山地景区", "cat", ["山地", "景区", "雪峰"]),
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
    ("shield", "安全防护", "svc", ["安全", "防护", "防卫"]),
    ("siren", "应急警报", "svc", ["应急", "警报", "突发"]),
    ("calendar", "排期预约", "svc", ["排期", "预约", "计划"]),
    ("pen-tool", "设计定制", "svc", ["设计", "定制", "方案"]),
    ("settings-2", "系统调试", "svc", ["调试", "系统", "参数"]),
    ("refresh-cw", "循环运维", "svc", ["循环", "运维", "更新"]),
    ("trending-up", "增效增长", "svc", ["增长", "增效", "提升"]),
    ("route", "路线规划", "svc", ["路线", "规划", "路径"]),
    ("paint-roller", "装饰装修", "svc", ["装饰", "装修", "涂装"]),
    ("wifi", "智能网络", "svc", ["智能", "网络", "无线"]),
    ("server", "机房运维", "svc", ["机房", "运维", "服务器"]),
    ("database", "数据服务", "svc", ["数据", "存储", "管理"]),
    ("monitor", "监控值守", "svc", ["监控", "值守", "大屏"]),
    ("thermometer", "温控调节", "svc", ["温控", "调节", "温度"]),
    ("fan", "通风换气", "svc", ["通风", "换气", "新风"]),
    ("snowflake", "制冷冷藏", "svc", ["制冷", "冷藏", "冷链"]),
    ("lock", "安全锁护", "svc", ["安全", "锁定", "防护"]),
    ("fingerprint", "身份核验", "svc", ["身份", "核验", "认证"]),
    ("credit-card", "支付结算", "svc", ["支付", "结算", "收款"]),
    ("receipt", "票据凭证", "svc", ["票据", "凭证", "回单"]),
    ("hourglass", "时效承诺", "svc", ["时效", "承诺", "工期"]),
    ("megaphone", "品牌传播", "svc", ["品牌", "传播", "推广"]),
    # ── 通用 ──
    ("image", "图片", "misc", ["图片", "照片", "image"]),
    ("users", "人群团队", "misc", ["团队", "人群", "用户"]),
    ("award", "荣誉奖项", "misc", ["荣誉", "奖项", "奖杯"]),
    ("gem", "精品案例", "misc", ["精品", "案例", "宝石"]),
    ("lightbulb", "方案创意", "misc", ["创意", "方案", "点子"]),
    ("map-pin", "位置服务", "misc", ["位置", "地图", "定位"]),
    ("globe", "全球业务", "misc", ["全球", "国际", "业务"]),
    ("qr-code", "扫码入口", "misc", ["扫码", "二维码", "入口"]),
    ("share-2", "分享传播", "misc", ["分享", "传播", "转发"]),
    ("message-circle", "消息沟通", "misc", ["消息", "沟通", "对话"]),
    ("thumbs-up", "口碑好评", "misc", ["好评", "口碑", "点赞"]),
    ("target", "目标达成", "misc", ["目标", "达成", "聚焦"]),
    ("compass", "方向指引", "misc", ["方向", "指引", "导航"]),
    ("layout-grid", "矩阵陈列", "misc", ["矩阵", "陈列", "网格"]),
    ("camera", "影像拍摄", "misc", ["影像", "拍摄", "摄影"]),
    ("printer", "印刷输出", "misc", ["印刷", "输出", "打印"]),
    ("shirt", "服饰定制", "misc", ["服饰", "定制", "服装"]),
    ("lamp", "照明亮化", "misc", ["照明", "亮化", "灯具"]),
    ("coffee", "商务洽谈", "misc", ["洽谈", "商务", "会客"]),
    ("cake", "庆典活动", "misc", ["庆典", "活动", "仪式"]),
    ("music", "文娱演出", "misc", ["文娱", "演出", "音乐"]),
    ("film", "影视制作", "misc", ["影视", "制作", "影片"]),
    ("palette", "创意设计", "misc", ["设计", "创意", "美术"]),
    ("gift", "礼遇回馈", "misc", ["礼遇", "回馈", "福利"]),
    ("box", "产品包装", "misc", ["产品", "包装", "货品"]),
    ("tag", "标签分类", "misc", ["标签", "分类", "标记"]),
]

# 封面：渐变 + 几何 motif，无文字。
# 四元组 (id, 名称, 上下渐变色, motif 名)：motif 与 id 解耦，
# 同一 motif 换配色即可派生新款式（扩充素材时只需加一行）。
COVERS = [
    ("cover-skyline", "天际线", ((11, 61, 145), (21, 101, 192)), "skyline"),
    ("cover-dots", "点阵", ((67, 56, 202), (99, 102, 241)), "dots"),
    ("cover-beams", "光束", ((15, 118, 110), (20, 184, 166)), "beams"),
    ("cover-arcs", "弧光", ((109, 40, 217), (139, 92, 246)), "arcs"),
    ("cover-blocks", "矩阵", ((51, 65, 85), (100, 116, 139)), "blocks"),
    ("cover-shield", "守护", ((21, 128, 61), (34, 197, 94)), "shield"),
    ("cover-motion", "疾驰", ((234, 88, 12), (249, 115, 22)), "motion"),
    ("cover-stars", "星网", ((15, 23, 42), (30, 58, 138)), "stars"),
    ("cover-waves", "波浪", ((14, 116, 144), (6, 182, 212)), "waves"),
    ("cover-hex", "蜂巢", ((190, 18, 60), (244, 63, 94)), "hex"),
    ("cover-mountain", "山峦", ((30, 41, 59), (71, 85, 105)), "mountain"),
    ("cover-bokeh", "光斑", ((49, 46, 129), (129, 60, 160)), "bokeh"),
    ("cover-gridline", "格线", ((17, 24, 39), (55, 65, 81)), "gridline"),
    ("cover-goldline", "金绶", ((20, 24, 33), (120, 53, 15)), "goldline"),
    ("cover-minimal", "极简", ((71, 85, 105), (148, 163, 184)), "minimal"),
    ("cover-night", "夜幕", ((2, 6, 23), (30, 27, 75)), "night"),
    # ── 新 motif ──
    ("cover-circuit", "电路", ((8, 47, 73), (2, 132, 199)), "circuit"),
    ("cover-rings", "同心环", ((76, 29, 149), (147, 51, 234)), "rings"),
    ("cover-stripes", "斜纹带", ((120, 53, 15), (217, 119, 6)), "stripes"),
    ("cover-bubbles", "气泡", ((6, 95, 108), (34, 211, 238)), "bubbles"),
    ("cover-cityline", "城市线", ((30, 27, 75), (79, 70, 229)), "cityline"),
    ("cover-veins", "叶脉", ((22, 101, 52), (74, 222, 128)), "veins"),
    ("cover-prism", "棱镜", ((157, 23, 77), (251, 113, 133)), "prism"),
    ("cover-topo", "等高线", ((17, 94, 89), (45, 212, 191)), "topo"),
    # ── 同 motif 换配色（同一款式满足不同品牌调性）──
    ("cover-skyline-warm", "暖阳天际线", ((120, 53, 15), (234, 88, 12)), "skyline"),
    ("cover-waves-deep", "深海波浪", ((12, 74, 110), (8, 145, 178)), "waves"),
    ("cover-hex-steel", "钢蓝蜂巢", ((51, 65, 85), (148, 163, 184)), "hex"),
    ("cover-minimal-sand", "沙调极简", ((120, 113, 108), (214, 211, 209)), "minimal"),
    ("cover-stars-dawn", "晨曦星网", ((76, 29, 149), (236, 72, 153)), "stars"),
    ("cover-blocks-mint", "薄荷矩阵", ((15, 118, 110), (94, 234, 212)), "blocks"),
    ("cover-arcs-rose", "玫红弧光", ((136, 19, 55), (244, 114, 182)), "arcs"),
    ("cover-goldline-emerald", "翡翠金绶", ((6, 78, 59), (16, 185, 129)), "goldline"),
]

# LOGO：渐变圆角方块 + 白色几何标记。
# 四元组 (id, 名称, 上下渐变色, mark 名)：mark 与 id 解耦，同款换色即可派生新 LOGO。
LOGOS = [
    ("logo-building", "建筑", ((26, 140, 255), (14, 111, 224)), "building"),
    ("logo-bolt", "闪电", ((245, 158, 11), (217, 119, 6)), "bolt"),
    ("logo-gear", "齿轮", ((71, 85, 105), (30, 41, 59)), "gear"),
    ("logo-shield", "盾牌", ((21, 128, 61), (22, 101, 52)), "shield"),
    ("logo-hex", "六边", ((109, 40, 217), (91, 33, 182)), "hex"),
    ("logo-arrow", "上升", ((20, 184, 166), (15, 118, 110)), "arrow"),
    ("logo-tower", "塔楼", ((91, 108, 255), (61, 79, 224)), "tower"),
    ("logo-orbit", "环轨", ((244, 63, 94), (190, 18, 60)), "orbit"),
    ("logo-diamond", "菱钻", ((14, 165, 233), (2, 132, 199)), "diamond"),
    ("logo-ringstar", "星环", ((234, 179, 8), (180, 83, 9)), "ringstar"),
    ("logo-peak", "山峰", ((16, 122, 87), (6, 78, 59)), "peak"),
    ("logo-flow", "流动", ((99, 102, 241), (67, 56, 202)), "flow"),
    ("logo-link", "链接", ((100, 116, 139), (51, 65, 85)), "link"),
    ("logo-cube", "立方", ((225, 29, 72), (159, 18, 57)), "cube"),
    # ── 新 mark ──
    ("logo-wave", "波浪", ((6, 182, 212), (14, 116, 144)), "wave"),
    ("logo-flame", "火焰", ((239, 68, 68), (185, 28, 28)), "flame"),
    ("logo-leaf", "叶片", ((34, 197, 94), (21, 128, 61)), "leaf"),
    ("logo-drop", "水滴", ((59, 130, 246), (29, 78, 216)), "drop"),
    ("logo-crown", "冠冕", ((217, 119, 6), (161, 98, 7)), "crown"),
    ("logo-spiral", "螺旋", ((139, 92, 246), (91, 33, 182)), "spiral"),
    # ── 同 mark 换配色 ──
    ("logo-bolt-cyan", "闪电 · 青", ((34, 211, 238), (8, 145, 178)), "bolt"),
    ("logo-gear-gold", "齿轮 · 金", ((234, 179, 8), (161, 98, 7)), "gear"),
    ("logo-shield-sky", "盾牌 · 天蓝", ((56, 189, 248), (2, 132, 199)), "shield"),
    ("logo-hex-lime", "六边 · 青柠", ((163, 230, 53), (77, 148, 60)), "hex"),
    ("logo-orbit-violet", "环轨 · 紫", ((167, 139, 250), (109, 40, 217)), "orbit"),
    ("logo-tower-slate", "塔楼 · 石板", ((148, 163, 184), (51, 65, 85)), "tower"),
]

COVER_W, COVER_H = 960, 504
THUMB_W, THUMB_H = 480, 252

# 小程序首页背景图：750×1334 竖幅（750rpx 设计稿 2x），深色高端商务风
BG_W, BG_H = 750, 1334
BG_THUMB_W, BG_THUMB_H = 300, 534

# (id, 名称, 上下渐变色, motif 名, 搜索标签)
# motif 与 id 解耦：同款构图换配色即可派生新背景（浅色系供浅色品牌调性选用）。
BACKGROUNDS = [
    ("bg-deepspace", "深空", ((8, 15, 40), (23, 37, 84)), "stars", ["深空", "星空", "商务蓝"]),
    ("bg-mountain", "山峦", ((15, 32, 46), (30, 66, 82)), "mountain", ["山峦", "层叠", "沉稳"]),
    ("bg-citynight", "城市夜景", ((10, 10, 30), (49, 27, 90)), "city", ["城市", "夜景", "楼宇"]),
    ("bg-bokeh", "光斑", ((30, 27, 75), (76, 29, 110)), "bokeh", ["光斑", "虚化", "梦幻"]),
    ("bg-waves", "涟漪", ((8, 51, 68), (14, 116, 144)), "waves", ["涟漪", "水波", "清新"]),
    ("bg-grid", "格线", ((15, 23, 42), (30, 41, 59)), "grid", ["格线", "科技", "理性"]),
    ("bg-diagonal", "斜纹", ((23, 37, 84), (30, 58, 138)), "diagonal", ["斜纹", "条纹", "简约"]),
    ("bg-glow", "光晕", ((30, 27, 75), (67, 56, 202)), "glow", ["光晕", "聚焦", "高级"]),
    ("bg-gold", "金线", ((12, 15, 23), (66, 48, 20)), "gold", ["金线", "奢华", "尊贵"]),
    ("bg-mesh", "星链", ((6, 46, 53), (15, 118, 110)), "mesh", ["星链", "节点", "网络"]),
    ("bg-aurora", "极光", ((6, 48, 60), (13, 90, 92)), "aurora", ["极光", "光带", "灵动"]),
    ("bg-sand", "流沙", ((41, 37, 36), (87, 73, 60)), "sand", ["流沙", "暖沙", "质感"]),
    # ── 新 motif ──
    ("bg-canyon", "峡谷", ((69, 26, 24), (154, 52, 18)), "canyon", ["峡谷", "岩壁", "暖调"]),
    ("bg-forest", "森林", ((10, 38, 28), (22, 78, 56)), "forest", ["森林", "树木", "自然"]),
    ("bg-ink", "水墨", ((228, 228, 231), (161, 161, 170)), "ink", ["水墨", "留白", "浅色"]),
    ("bg-geo", "几何", ((30, 58, 138), (12, 74, 110)), "geo", ["几何", "拼接", "现代"]),
    ("bg-lines", "光束线", ((15, 23, 42), (55, 65, 81)), "lines", ["光束", "细线", "秩序"]),
    ("bg-nebula", "星云", ((23, 37, 84), (76, 29, 149)), "nebula", ["星云", "梦幻", "渐变"]),
    ("bg-noir", "暗夜", ((9, 9, 11), (39, 39, 42)), "noir", ["暗夜", "极简", "克制"]),
    ("bg-ocean", "深海", ((8, 47, 73), (12, 74, 110)), "ocean", ["深海", "光束", "静谧"]),
    ("bg-pearl", "珠光", ((243, 244, 246), (203, 213, 225)), "pearl", ["珠光", "浅色", "柔和"]),
    ("bg-ridge", "山脊", ((30, 41, 59), (51, 65, 85)), "ridge", ["山脊", "细线", "冷峻"]),
    ("bg-tech", "科技", ((12, 74, 110), (2, 132, 199)), "tech", ["科技", "电路", "智能"]),
    ("bg-linen", "亚麻", ((231, 229, 228), (168, 162, 158)), "linen", ["亚麻", "暖灰", "浅色"]),
    # ── 同 motif 换配色 ──
    ("bg-dusk", "暮色", ((76, 29, 149), (190, 24, 93)), "mountain", ["暮色", "紫粉", "渐变"]),
    ("bg-mint", "薄荷", ((209, 250, 229), (94, 234, 212)), "waves", ["薄荷", "清新", "浅色"]),
    ("bg-coral", "珊瑚", ((127, 29, 29), (225, 29, 72)), "bokeh", ["珊瑚", "热情", "暖调"]),
    ("bg-royal", "皇家蓝", ((12, 35, 128), (29, 78, 216)), "glow", ["皇家蓝", "深邃", "大气"]),
    ("bg-charcoal", "炭黑", ((24, 24, 27), (63, 63, 70)), "grid", ["炭黑", "沉稳", "中性"]),
    ("bg-amber", "琥珀", ((254, 243, 199), (217, 119, 6)), "diagonal", ["琥珀", "暖阳", "浅色"]),
]

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
    ("avatar-9", "默认头像 · 金", ((234, 179, 8), (180, 83, 9))),
    ("avatar-10", "默认头像 · 深蓝", ((30, 64, 175), (30, 58, 138))),
    ("avatar-11", "默认头像 · 青碧", ((13, 148, 136), (15, 118, 110))),
    ("avatar-12", "默认头像 · 绛红", ((190, 24, 93), (157, 23, 77))),
    ("avatar-13", "默认头像 · 藏青", ((30, 58, 138), (23, 37, 84))),
    ("avatar-14", "默认头像 · 琥珀", ((217, 119, 6), (161, 98, 7))),
    ("avatar-15", "默认头像 · 石板", ((71, 85, 105), (30, 41, 59))),
    ("avatar-16", "默认头像 · 薄荷", ((94, 234, 212), (13, 148, 136))),
    ("avatar-17", "默认头像 · 珊瑚", ((251, 113, 133), (225, 29, 72))),
    ("avatar-18", "默认头像 · 靛紫", ((129, 140, 248), (67, 56, 202))),
    ("avatar-19", "默认头像 · 橄榄", ((132, 204, 22), (63, 98, 18))),
    ("avatar-20", "默认头像 · 酒红", ((153, 27, 27), (127, 29, 29))),
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


def motif_mountain(img):
    """层叠山脉剪影（远山浅近山深，稳重商务）。"""
    ov, d = overlay(img)
    w, h = img.size
    layers = [(0.52, 60, 0.62), (0.66, 90, 0.78), (0.80, 130, 1.0)]
    random.seed(57)
    for base_y, alpha, _ in layers:
        pts = [(0, h)]
        x = 0
        peak = h * base_y
        while x < w:
            step = random.randint(90, 170)
            ph = random.randint(40, 130)
            pts += [(x + step / 2, peak - ph), (x + step, peak)]
            x += step
        pts += [(w, h)]
        d.polygon(pts, fill=(255, 255, 255, alpha))
    img.alpha_composite(ov)


def motif_bokeh(img):
    """柔和光斑（大小不一的半透明圆，梦幻高级）。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(63)
    for _ in range(26):
        r = random.randint(26, 96)
        x = random.randint(-40, w + 40)
        y = random.randint(-40, int(h * 0.9))
        a = random.randint(18, 70)
        d.ellipse([x - r, y - r, x + r, y + r], fill=(255, 255, 255, a))
        d.ellipse([x - r, y - r, x + r, y + r], outline=(255, 255, 255, a + 40), width=2)
    img.alpha_composite(ov)


def motif_gridline(img):
    """细网格 + 高亮节点（科技理性）。"""
    ov, d = overlay(img)
    w, h = img.size
    step = 72
    for x in range(0, w + 1, step):
        d.line([(x, 0), (x, h)], fill=(255, 255, 255, 34), width=1)
    for y in range(0, h + 1, step):
        d.line([(0, y), (w, y)], fill=(255, 255, 255, 34), width=1)
    for cx, cy in ((w * 0.24, h * 0.30), (w * 0.68, h * 0.56), (w * 0.42, h * 0.80)):
        r = 10
        d.ellipse([cx - r, cy - r, cx + r, cy + r], fill=(255, 255, 255, 150))
        d.ellipse([cx - 22, cy - 22, cx + 22, cy + 22],
                  outline=(255, 255, 255, 90), width=2)
    img.alpha_composite(ov)


def motif_goldline(img):
    """金色装饰线框（奢华尊贵，深底金字）。"""
    ov, d = overlay(img)
    w, h = img.size
    gold = (212, 175, 55, 200)
    m = 44
    d.rectangle([m, m, w - m, h - m], outline=gold, width=3)
    d.rectangle([m + 12, m + 12, w - m - 12, h - m - 12],
                outline=(212, 175, 55, 90), width=1)
    cx, cy = w / 2, h / 2
    d.line([(cx - 90, cy), (cx + 90, cy)], fill=gold, width=3)
    d.ellipse([cx - 8, cy - 8, cx + 8, cy + 8], fill=gold)
    img.alpha_composite(ov)


def motif_minimal(img):
    """极简：一个大圆环 + 一根细线（大量留白）。"""
    ov, d = overlay(img)
    w, h = img.size
    cx, cy, r = w * 0.5, h * 0.44, min(w, h) * 0.30
    d.ellipse([cx - r, cy - r, cx + r, cy + r],
              outline=(255, 255, 255, 170), width=8)
    d.line([(w * 0.18, h * 0.80), (w * 0.82, h * 0.80)],
           fill=(255, 255, 255, 110), width=3)
    img.alpha_composite(ov)


def motif_night(img):
    """夜幕楼宇：黑色楼体剪影 + 窗格灯火。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(71)
    x = -20
    while x < w + 20:
        bw = random.randint(70, 130)
        bh = random.randint(int(h * 0.28), int(h * 0.62))
        top = h - bh
        d.rectangle([x, top, x + bw, h], fill=(0, 0, 0, 120))
        for wy in range(top + 16, h - 14, 26):
            for wx in range(x + 12, x + bw - 10, 24):
                if random.random() < 0.42:
                    d.rectangle([wx, wy, wx + 10, wy + 12],
                                fill=(255, 214, 130, random.randint(120, 220)))
        x += bw + random.randint(10, 30)
    img.alpha_composite(ov)


def motif_circuit(img):
    """电路走线：直角折线 + 焊点（科技/智造）。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(131)
    for _ in range(14):
        x, y = random.randint(0, w), random.randint(0, h)
        pts = [(x, y)]
        for _ in range(random.randint(2, 4)):
            if random.random() < 0.5:
                x += random.choice([-1, 1]) * random.randint(60, 180)
            else:
                y += random.choice([-1, 1]) * random.randint(40, 120)
            pts.append((x, y))
        d.line(pts, fill=(255, 255, 255, random.randint(50, 130)),
               width=3, joint="curve")
        d.ellipse([pts[-1][0] - 6, pts[-1][1] - 6, pts[-1][0] + 6, pts[-1][1] + 6],
                  fill=(255, 255, 255, 190))
    img.alpha_composite(ov)


def motif_rings(img):
    """同心环：右下角发散圆环。"""
    ov, d = overlay(img)
    w, h = img.size
    cx, cy = w * 0.74, h * 0.60
    for i, r in enumerate(range(70, 700, 62)):
        d.ellipse([cx - r, cy - r, cx + r, cy + r],
                  outline=(255, 255, 255, 46 + (i % 4) * 26), width=4)
    img.alpha_composite(ov)


def motif_stripes(img):
    """斜纹带：不等宽斜条（动感）。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(133)
    x = -h
    while x < w + h:
        bw = random.choice([26, 42, 64]) if random.random() < 0.5 else 12
        d.polygon([(x, 0), (x + 90, 0), (x + 90 + h, h), (x + h, h)],
                  fill=(255, 255, 255, random.randint(24, 96)))
        x += bw + 34
    img.alpha_composite(ov)


def motif_bubbles(img):
    """气泡：大小圆错落悬浮。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(137)
    for _ in range(34):
        r = random.randint(16, 74)
        x, y = random.randint(0, w), random.randint(0, h)
        d.ellipse([x - r, y - r, x + r, y + r],
                  outline=(255, 255, 255, random.randint(50, 140)), width=3)
        if r > 44:
            d.ellipse([x - r * 0.42, y - r * 0.52, x - r * 0.14, y - r * 0.24],
                      fill=(255, 255, 255, 120))
    img.alpha_composite(ov)


def motif_cityline(img):
    """城市线：白色线描天际线（简约现代）。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(139)
    x = -20
    pts = [(0, h)]
    while x < w + 20:
        bw = random.randint(50, 110)
        bh = random.randint(60, 220)
        pts += [(x, h - bh), (x + bw, h - bh)]
        x += bw
    pts += [(w, h)]
    d.line(pts, fill=(255, 255, 255, 190), width=5, joint="curve")
    for i in range(9):
        y = h - random.randint(30, 200)
        d.line([(0, y), (w, y)], fill=(255, 255, 255, 26), width=2)
    img.alpha_composite(ov)


def motif_veins(img):
    """叶脉：主脉 + 对称侧脉（生态/农业）。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(149)
    d.line([(w * 0.5, h), (w * 0.5, -20)], fill=(255, 255, 255, 150), width=6)
    for i in range(11):
        y = h - i * (h / 11)
        ln = 90 + i * 26
        for sign in (-1, 1):
            d.line([(w * 0.5, y), (w * 0.5 + sign * ln, y - 46)],
                   fill=(255, 255, 255, 70 + (i % 3) * 22), width=3)
    img.alpha_composite(ov)


def motif_prism(img):
    """棱镜：色散三角光带。"""
    ov, d = overlay(img)
    w, h = img.size
    cx, cy = w * 0.22, h * 0.52
    for i, col in enumerate(((255, 138, 138), (255, 214, 130),
                             (150, 255, 190), (150, 210, 255))):
        spread = 40 + i * 34
        d.polygon([(cx, cy), (w, cy - 90 - spread), (w, cy + 90 - spread)],
                  fill=col + (54,))
    d.polygon([(cx - 60, cy - 92), (cx + 60, cy - 92), (cx, cy + 96)],
              outline=(255, 255, 255, 220), width=4)
    img.alpha_composite(ov)


def motif_topo(img):
    """等高线：层叠闭合曲线（测绘/地理）。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(151)
    for i in range(7):
        pts = []
        for x in range(0, w + 20, 20):
            y = h * (0.22 + i * 0.10) + math.sin(x / 130 + i) * 34 + \
                math.sin(x / 47 + i * 2) * 14
            pts.append((x, y))
        d.line(pts, fill=(255, 255, 255, 40 + i * 14), width=3)
    img.alpha_composite(ov)


COVER_MOTIFS = {
    "skyline": motif_skyline, "dots": motif_dots, "beams": motif_beams,
    "arcs": motif_arcs, "blocks": motif_blocks, "shield": motif_shield,
    "motion": motif_motion, "stars": motif_stars, "waves": motif_waves,
    "hex": motif_hex, "mountain": motif_mountain, "bokeh": motif_bokeh,
    "gridline": motif_gridline, "goldline": motif_goldline,
    "minimal": motif_minimal, "night": motif_night, "circuit": motif_circuit,
    "rings": motif_rings, "stripes": motif_stripes, "bubbles": motif_bubbles,
    "cityline": motif_cityline, "veins": motif_veins, "prism": motif_prism,
    "topo": motif_topo,
}


def make_cover(item_id, name, colors, motif):
    img = vgrad((COVER_W, COVER_H), *colors).convert("RGBA")
    COVER_MOTIFS[motif](img)
    path = os.path.join(OUT, "covers", item_id + ".jpg")
    img.convert("RGB").save(path, "JPEG", quality=87)
    # 预览缩略图（选择器里加载，避免原图流量）
    thumb = img.convert("RGB").resize((THUMB_W, THUMB_H), Image.LANCZOS)
    tpath = os.path.join(OUT, "thumbs", "covers", item_id + ".jpg")
    thumb.save(tpath, "JPEG", quality=80)
    return {"id": item_id, "name": name, "category": "cover",
            "file": "covers/%s.jpg" % item_id, "thumb": "thumbs/covers/%s.jpg" % item_id,
            "width": COVER_W, "height": COVER_H,
            "tags": [name, "封面", "背景", motif]}


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


def mark_diamond(size):
    """菱钻：双层菱形 + 棱线。"""
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    cx, cy, s = size / 2, size * 0.48, size * 0.30
    pts = [(cx, cy - s), (cx + s * 0.72, cy), (cx, cy + s), (cx - s * 0.72, cy)]
    d.line(pts + [pts[0]], fill=(255, 255, 255, 255),
           width=int(size * 0.05), joint="curve")
    d.line([(cx, cy - s), (cx, cy + s)], fill=(255, 255, 255, 160),
           width=int(size * 0.03))
    d.line([(cx - s * 0.72, cy), (cx + s * 0.72, cy)],
           fill=(255, 255, 255, 160), width=int(size * 0.03))
    return lay


def mark_ringstar(size):
    """星环：五角星 + 外圈细环。"""
    import math
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    cx, cy, r = size / 2, size * 0.48, size * 0.26
    pts = []
    for i in range(10):
        rr = r if i % 2 == 0 else r * 0.42
        a = math.radians(-90 + i * 36)
        pts.append((cx + rr * math.cos(a), cy + rr * math.sin(a)))
    d.polygon(pts, fill=(255, 255, 255, 255))
    d.ellipse([cx - size * 0.34, cy - size * 0.34, cx + size * 0.34, cy + size * 0.34],
              outline=(255, 255, 255, 200), width=int(size * 0.035))
    return lay


def mark_peak(size):
    """山峰：双峰 + 地平线。"""
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    d.line([(size * 0.20, size * 0.76), (size * 0.42, size * 0.34),
            (size * 0.58, size * 0.62), (size * 0.68, size * 0.48),
            (size * 0.84, size * 0.76)],
           fill=(255, 255, 255, 255), width=int(size * 0.05), joint="curve")
    d.line([(size * 0.16, size * 0.82), (size * 0.86, size * 0.82)],
           fill=(255, 255, 255, 255), width=int(size * 0.045))
    return lay


def mark_flow(size):
    """流动：三条错位弧线。"""
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    for i in range(3):
        y = size * (0.34 + i * 0.16)
        d.arc([size * 0.22, y - size * 0.10, size * 0.78, y + size * 0.10],
              200, 340, fill=(255, 255, 255, 255), width=int(size * 0.05))
    return lay


def mark_link(size):
    """链接：双环相扣。"""
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    r = size * 0.20
    d.ellipse([size * 0.20, size * 0.30, size * 0.20 + 2 * r, size * 0.30 + 2 * r],
              outline=(255, 255, 255, 255), width=int(size * 0.055))
    d.ellipse([size * 0.60 - 2 * r + 2 * r, size * 0.30,
               size * 0.60 + 2 * r, size * 0.30 + 2 * r],
              outline=(255, 255, 255, 255), width=int(size * 0.055))
    return lay


def mark_cube(size):
    """立方：等距立方体线框。"""
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    cx, cy, s = size / 2, size * 0.50, size * 0.26
    top = [(cx, cy - s), (cx + s, cy - s * 0.5), (cx, cy), (cx - s, cy - s * 0.5)]
    d.line(top + [top[0]], fill=(255, 255, 255, 255),
           width=int(size * 0.045), joint="curve")
    bot = [(cx, cy), (cx + s, cy + s * 0.5), (cx, cy + s), (cx - s, cy + s * 0.5)]
    d.line(bot + [bot[0]], fill=(255, 255, 255, 255),
           width=int(size * 0.045), joint="curve")
    d.line([(cx, cy - s), (cx, cy)], fill=(255, 255, 255, 200),
           width=int(size * 0.04))
    d.line([(cx + s, cy - s * 0.5), (cx + s, cy + s * 0.5)],
           fill=(255, 255, 255, 200), width=int(size * 0.04))
    d.line([(cx - s, cy - s * 0.5), (cx - s, cy + s * 0.5)],
           fill=(255, 255, 255, 200), width=int(size * 0.04))
    return lay


def mark_wave(size):
    """波浪：三条错位正弦线。"""
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    for i in range(3):
        y = size * (0.36 + i * 0.15)
        amp = size * 0.07
        pts = [(x, y + amp * math.sin(x / size * 6.0 + i)) for x in range(0, size + 4, 4)]
        d.line(pts, fill=(255, 255, 255, 255), width=int(size * 0.045), joint="curve")
    return lay


def mark_flame(size):
    """火焰：水滴形火苗 + 内焰。"""
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    cx, s = size / 2, size * 0.30
    outer = [(cx, size * 0.16), (cx + s * 0.78, size * 0.52),
             (cx + s * 0.62, size * 0.84), (cx, size * 0.86),
             (cx - s * 0.62, size * 0.84), (cx - s * 0.78, size * 0.52)]
    d.polygon(outer, fill=(255, 255, 255, 255))
    d.polygon([(cx, size * 0.44), (cx + s * 0.34, size * 0.66),
               (cx, size * 0.80), (cx - s * 0.34, size * 0.66)],
              fill=(0, 0, 0, 90))
    return lay


def mark_leaf(size):
    """叶片：对称叶形 + 主脉。"""
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    cx, cy, s = size / 2, size / 2, size * 0.30
    d.polygon([(cx - s * 0.85, cy + s * 0.85), (cx + s * 0.85, cy - s * 0.85),
               (cx + s * 0.85, cy + s * 0.20), (cx - s * 0.85, cy + s * 0.20)],
              fill=(255, 255, 255, 255))
    d.line([(cx - s * 0.85, cy + s * 0.85), (cx + s * 0.85, cy - s * 0.85)],
           fill=(0, 0, 0, 70), width=int(size * 0.035))
    return lay


def mark_drop(size):
    """水滴：圆底尖顶 + 高光。"""
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    cx, cy, s = size / 2, size * 0.54, size * 0.28
    d.polygon([(cx, cy - s), (cx + s * 0.86, cy + s * 0.30),
               (cx, cy + s), (cx - s * 0.86, cy + s * 0.30)],
              fill=(255, 255, 255, 255))
    d.ellipse([cx - s * 0.62, cy + s * 0.02, cx + s * 0.62, cy + s * 1.0],
              fill=(255, 255, 255, 255))
    d.ellipse([cx - s * 0.34, cy - s * 0.02, cx - s * 0.06, cy + s * 0.30],
              fill=(0, 0, 0, 60))
    return lay


def mark_crown(size):
    """冠冕：三尖王冠 + 底座。"""
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    w = int(size * 0.05)
    pts = [(size * 0.20, size * 0.72), (size * 0.24, size * 0.30),
           (size * 0.38, size * 0.52), (size * 0.50, size * 0.22),
           (size * 0.62, size * 0.52), (size * 0.76, size * 0.30),
           (size * 0.80, size * 0.72)]
    d.line(pts, fill=(255, 255, 255, 255), width=w, joint="curve")
    d.line([(size * 0.20, size * 0.72), (size * 0.80, size * 0.72)],
           fill=(255, 255, 255, 255), width=w)
    d.line([(size * 0.18, size * 0.84), (size * 0.82, size * 0.84)],
           fill=(255, 255, 255, 220), width=int(w * 0.8))
    return lay


def mark_spiral(size):
    """螺旋：等距阿基米德螺线（半径收在标记区内，不顶边）。"""
    lay = _mark_layer(size)
    d = ImageDraw.Draw(lay)
    cx, cy = size / 2, size / 2
    pts = []
    for i in range(240):
        t = i / 240 * math.pi * 6
        r = size * 0.04 + t * size * 0.017
        pts.append((cx + r * math.cos(t), cy + r * math.sin(t)))
    d.line(pts, fill=(255, 255, 255, 255), width=int(size * 0.045), joint="curve")
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
    "building": mark_building, "bolt": mark_bolt, "gear": mark_gear,
    "shield": mark_shield, "hex": mark_hex, "arrow": mark_arrow,
    "tower": mark_tower, "orbit": mark_orbit, "diamond": mark_diamond,
    "ringstar": mark_ringstar, "peak": mark_peak, "flow": mark_flow,
    "link": mark_link, "cube": mark_cube, "wave": mark_wave, "flame": mark_flame,
    "leaf": mark_leaf, "drop": mark_drop, "crown": mark_crown, "spiral": mark_spiral,
}


def make_logo(item_id, name, colors, mark):
    S = 512
    tile = rounded_tile(S, *colors, radius_ratio=0.22)
    tile.alpha_composite(LOGO_MARKS[mark](S), (0, 0))
    path = os.path.join(OUT, "logos", item_id + ".png")
    tile.save(path, "PNG")
    return {"id": item_id, "name": name, "category": "logo",
            "file": "logos/%s.png" % item_id, "thumb": "logos/%s.png" % item_id,
            "width": S, "height": S, "tags": [name, "LOGO", "标识", mark]}


# ── 小程序首页背景图（750×1334 竖幅，深色高端商务风） ───────────────────
def _radial_fade(size, cx, cy, radius, power=1.7, peak=255):
    """平滑径向衰减图（L 模式）：(cx,cy) 处 peak，radius 处 0。
    幂函数缓出保证边缘一阶导为 0（无圆形硬边）；低分辨率计算 + LANCZOS 放大，快且平滑。"""
    w, h = size
    sw = 128
    sh = max(8, int(sw * h / w))
    px = []
    for y in range(sh):
        dy = (y + 0.5) / sh * h - cy
        for x in range(sw):
            dx = (x + 0.5) / sw * w - cx
            t = min(1.0, (dx * dx + dy * dy) ** 0.5 / radius)
            px.append(int(peak * (1 - t) ** power))
    small = Image.new("L", (sw, sh))
    small.putdata(px)
    return small.resize((w, h), Image.LANCZOS)


def _radial_glow(size, center, radius, color=(255, 255, 255), max_alpha=90):
    """径向光晕层：center 最亮，向边缘平滑消失（无硬边）。"""
    w, h = size
    a = _radial_fade(size, center[0], center[1], radius)
    a = a.point(lambda v: int(v * max_alpha / 255))
    glow = Image.new("RGBA", (w, h), color + (0,))
    glow.putalpha(a)
    return glow


def _vignette(img, strength=64):
    """四角暗角：中心亮、边缘暗，椭圆衰减 + 幂缓出，无环形接缝。"""
    w, h = img.size
    a = _radial_fade(img.size, w * 0.5, h * 0.5, max(w, h) * 0.80)
    a = a.point(lambda v: int((255 - v) * strength / 255))
    black = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    black.putalpha(a)
    img.alpha_composite(black)


def bg_stars(img):
    """深空：上段星云光晕 + 随机星点。"""
    w, h = img.size
    img.alpha_composite(_radial_glow(img.size, (w * 0.5, h * 0.14), w * 0.95,
                                     (96, 165, 250), 66))
    ov, d = overlay(img)
    random.seed(101)
    for _ in range(120):
        x, y = random.randint(0, w), random.randint(0, int(h * 0.75))
        r = random.choice([1, 1, 2, 2, 3])
        d.ellipse([x - r, y - r, x + r, y + r],
                  fill=(255, 255, 255, random.randint(70, 210)))
    img.alpha_composite(ov)


def bg_mountain(img):
    """山峦：下段三层山脉剪影。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(103)
    for base, alpha in ((0.66, 70), (0.76, 100), (0.86, 140)):
        pts = [(0, h)]
        x, peak = 0, h * base
        while x < w:
            step = random.randint(120, 220)
            ph = random.randint(60, 200)
            pts += [(x + step / 2, peak - ph), (x + step, peak)]
            x += step
        pts += [(w, h)]
        d.polygon(pts, fill=(255, 255, 255, alpha))
    img.alpha_composite(ov)


def bg_city(img):
    """城市夜景：底部楼宇剪影 + 暖色窗格灯火。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(107)
    x = -30
    while x < w + 30:
        bw = random.randint(90, 170)
        bh = random.randint(int(h * 0.20), int(h * 0.46))
        top = h - bh
        d.rectangle([x, top, x + bw, h], fill=(0, 0, 0, 130))
        for wy in range(top + 18, h - 16, 30):
            for wx in range(x + 14, x + bw - 12, 28):
                if random.random() < 0.38:
                    d.rectangle([wx, wy, wx + 12, wy + 14],
                                fill=(255, 214, 130, random.randint(110, 220)))
        x += bw + random.randint(12, 34)
    img.alpha_composite(ov)


def bg_bokeh(img):
    """光斑：大幅面柔光圆斑。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(109)
    for _ in range(22):
        r = random.randint(40, 150)
        x = random.randint(-60, w + 60)
        y = random.randint(-40, int(h * 0.85))
        a = random.randint(14, 52)
        d.ellipse([x - r, y - r, x + r, y + r], fill=(255, 255, 255, a))
        d.ellipse([x - r, y - r, x + r, y + r],
                  outline=(255, 255, 255, a + 26), width=3)
    img.alpha_composite(ov)


def bg_waves(img):
    """涟漪：同心弧线自下而上展开。"""
    ov, d = overlay(img)
    w, h = img.size
    cx, cy = w * 0.5, h * 1.02
    for i, r in enumerate(range(140, 1150, 95)):
        d.arc([cx - r, cy - r, cx + r, cy + r], 200, 340,
              fill=(255, 255, 255, 46 + (i % 3) * 26), width=6)
    img.alpha_composite(ov)


def bg_grid(img):
    """格线：下段透视网格 + 地平线。"""
    ov, d = overlay(img)
    w, h = img.size
    horizon = int(h * 0.58)
    for i in range(-10, 11):
        d.line([(w / 2 + i * 40, horizon), (w / 2 + i * 300, h)],
               fill=(255, 255, 255, 40), width=2)
    for j in range(1, 9):
        y = horizon + (h - horizon) * (j / 9) ** 1.6
        d.line([(0, y), (w, y)], fill=(255, 255, 255, 40), width=2)
    d.line([(0, horizon), (w, horizon)], fill=(255, 255, 255, 110), width=2)
    img.alpha_composite(ov)


def bg_diagonal(img):
    """斜纹：细密斜线条纹。"""
    ov, d = overlay(img)
    w, h = img.size
    gap, width = 46, 10
    x = -h
    while x < w + h:
        d.line([(x, 0), (x + h, h)], fill=(255, 255, 255, 30), width=width)
        d.line([(x + gap, 0), (x + gap + h, h)], fill=(255, 255, 255, 14), width=width)
        x += gap * 2
    img.alpha_composite(ov)


def bg_glow(img):
    """光晕：中央强径向光 + 底部辅光。"""
    w, h = img.size
    img.alpha_composite(_radial_glow(img.size, (w * 0.5, h * 0.32), w * 1.0,
                                     (165, 180, 252), 96))
    img.alpha_composite(_radial_glow(img.size, (w * 0.5, h * 0.94), w * 0.8,
                                     (99, 102, 241), 52))


def bg_gold(img):
    """金线：金色细框 + 竖向金线（奢华）。"""
    ov, d = overlay(img)
    w, h = img.size
    gold = (212, 175, 55, 210)
    m = 40
    d.rectangle([m, m, w - m, h - m], outline=gold, width=3)
    d.rectangle([m + 14, m + 14, w - m - 14, h - m - 14],
                outline=(212, 175, 55, 80), width=1)
    for x in (w * 0.5,):
        d.line([(x, m + 40), (x, h - m - 40)], fill=(212, 175, 55, 60), width=2)
    d.ellipse([w * 0.5 - 10, h * 0.30 - 10, w * 0.5 + 10, h * 0.30 + 10], fill=gold)
    img.alpha_composite(ov)


def bg_mesh(img):
    """星链：节点 + 邻近连线（网络感）。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(127)
    nodes = [(random.randint(30, w - 30), random.randint(60, int(h * 0.8)))
             for _ in range(26)]
    for i, (x1, y1) in enumerate(nodes):
        for x2, y2 in nodes[i + 1:]:
            dist2 = (x1 - x2) ** 2 + (y1 - y2) ** 2
            if dist2 < 260 ** 2:
                d.line([(x1, y1), (x2, y2)], fill=(255, 255, 255, 54), width=2)
    for x, y in nodes:
        r = random.choice([4, 5, 6])
        d.ellipse([x - r, y - r, x + r, y + r],
                  fill=(255, 255, 255, random.randint(120, 220)))
    img.alpha_composite(ov)


def bg_aurora(img):
    """极光：两条斜向柔光带。"""
    w, h = img.size
    for cx, cy, color, alpha in (
        (w * 0.30, h * 0.28, (94, 234, 212), 78),
        (w * 0.72, h * 0.50, (56, 189, 248), 62),
    ):
        band = Image.new("RGBA", (w, h), (0, 0, 0, 0))
        bd = ImageDraw.Draw(band)
        bd.line([(cx - w * 0.5, cy + h * 0.24), (cx + w * 0.5, cy - h * 0.24)],
                fill=color + (alpha,), width=int(h * 0.20))
        band = band.filter(ImageFilter.GaussianBlur(80))
        img.alpha_composite(band)


def bg_sand(img):
    """流沙：层叠沙丘曲线。"""
    ov, d = overlay(img)
    w, h = img.size
    for base, alpha in ((0.62, 46), (0.74, 70), (0.88, 100)):
        pts = [(0, h)]
        x = 0
        while x <= w:
            t = x / w
            y = h * base - math.sin(t * math.pi * 1.6 + base * 9) * h * 0.05
            pts.append((x, y))
            x += 30
        pts += [(w, h)]
        d.polygon(pts, fill=(255, 255, 255, alpha))
    img.alpha_composite(ov)


def bg_canyon(img):
    """峡谷：两侧暖色岩壁夹一道光。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(157)
    for side in (0, 1):
        pts = [(0 if side == 0 else w, 0)]
        for i in range(9):
            y = i * h / 8
            inset = random.randint(60, 230) if side == 0 else w - random.randint(60, 230)
            pts.append((inset, y))
        pts.append((0 if side == 0 else w, h))
        d.polygon(pts, fill=(0, 0, 0, 120))
    img.alpha_composite(ov)
    img.alpha_composite(_radial_glow(img.size, (w * 0.5, h * 0.30), w * 0.9,
                                     (255, 214, 170), 70))


def bg_forest(img):
    """森林：深浅两层树干剪影 + 顶部透光。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(163)
    for depth, alpha, wdt in ((0, 150, (26, 46)), (1, 90, (14, 28))):
        x = -40
        while x < w + 40:
            bw = random.randint(*wdt)
            d.rectangle([x, 0, x + bw, h], fill=(0, 0, 0, alpha))
            x += random.randint(90, 210)
    img.alpha_composite(ov)
    img.alpha_composite(_radial_glow(img.size, (w * 0.5, h * 0.06), w * 1.0,
                                     (187, 247, 208), 54))


def bg_ink(img):
    """水墨（浅色）：淡墨晕染 + 飞白点，留白充足。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(167)
    for _ in range(7):
        r = random.randint(90, 260)
        x, y = random.randint(0, w), random.randint(int(h * 0.15), int(h * 0.85))
        d.ellipse([x - r, y - r * 0.7, x + r, y + r * 0.7],
                  fill=(39, 39, 42, random.randint(14, 34)))
    for _ in range(60):
        x, y = random.randint(0, w), random.randint(0, h)
        r = random.randint(1, 4)
        d.ellipse([x - r, y - r, x + r, y + r], fill=(39, 39, 42, random.randint(40, 120)))
    img.alpha_composite(ov)


def bg_geo(img):
    """几何：大块半透明多边形拼接。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(173)
    for _ in range(9):
        cx, cy = random.randint(0, w), random.randint(0, h)
        r = random.randint(160, 380)
        n = random.randint(3, 6)
        pts = [(cx + r * math.cos(math.radians(360 / n * i + random.randint(0, 60))),
                cy + r * math.sin(math.radians(360 / n * i + random.randint(0, 60))))
               for i in range(n)]
        d.polygon(pts, fill=(255, 255, 255, random.randint(16, 46)))
    img.alpha_composite(ov)


def bg_lines(img):
    """光束线：横向细线自上而下渐隐。"""
    ov, d = overlay(img)
    w, h = img.size
    for i in range(46):
        y = int(h * 0.10 + i * (h * 0.9 / 46))
        a = int(90 * (1 - i / 46) ** 1.4)
        d.line([(0, y), (w, y)], fill=(255, 255, 255, a), width=2)
    img.alpha_composite(ov)


def bg_nebula(img):
    """星云：多团彩色光晕叠加 + 细星点。"""
    w, h = img.size
    for cx, cy, r, col, a in (
        (w * 0.30, h * 0.26, w * 0.80, (129, 140, 248), 60),
        (w * 0.72, h * 0.48, w * 0.72, (244, 114, 182), 50),
        (w * 0.44, h * 0.76, w * 0.66, (56, 189, 248), 44),
    ):
        img.alpha_composite(_radial_glow(img.size, (cx, cy), r, col, a))
    ov, d = overlay(img)
    random.seed(179)
    for _ in range(150):
        x, y = random.randint(0, w), random.randint(0, h)
        r = random.choice([1, 1, 2])
        d.ellipse([x - r, y - r, x + r, y + r],
                  fill=(255, 255, 255, random.randint(60, 190)))
    img.alpha_composite(ov)


def bg_noir(img):
    """暗夜：近黑底 + 单点柔光（极简克制）。"""
    w, h = img.size
    img.alpha_composite(_radial_glow(img.size, (w * 0.5, h * 0.34), w * 0.85,
                                     (228, 228, 231), 46))


def bg_ocean(img):
    """深海：自上而下的光束 + 悬浮微粒。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(181)
    for _ in range(9):
        x = random.randint(0, w)
        wid = random.randint(40, 130)
        d.polygon([(x, 0), (x + wid, 0), (x + wid + 180, h), (x + 150, h)],
                  fill=(255, 255, 255, random.randint(14, 40)))
    for _ in range(70):
        x, y = random.randint(0, w), random.randint(0, h)
        r = random.randint(1, 3)
        d.ellipse([x - r, y - r, x + r, y + r],
                  fill=(255, 255, 255, random.randint(50, 140)))
    img.alpha_composite(ov)


def bg_pearl(img):
    """珠光（浅色）：柔和彩虹光晕 + 细腻珠光斑。"""
    w, h = img.size
    for cx, cy, r, col, a in (
        (w * 0.28, h * 0.24, w * 0.85, (244, 114, 182), 30),
        (w * 0.74, h * 0.52, w * 0.85, (56, 189, 248), 26),
        (w * 0.42, h * 0.84, w * 0.85, (167, 139, 250), 24),
    ):
        img.alpha_composite(_radial_glow(img.size, (cx, cy), r, col, a))


def bg_ridge(img):
    """山脊：细线层叠山脊（冷峻理性）。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(191)
    for i in range(6):
        base = h * (0.40 + i * 0.09)
        pts = []
        for x in range(0, w + 24, 24):
            y = base - abs(math.sin(x / 190 + i * 1.3)) * h * 0.10 - \
                random.randint(0, 16)
            pts.append((x, y))
        d.line(pts, fill=(255, 255, 255, 36 + i * 16), width=3)
    img.alpha_composite(ov)


def bg_tech(img):
    """科技：电路板走线 + 节点。"""
    ov, d = overlay(img)
    w, h = img.size
    random.seed(193)
    for _ in range(18):
        x, y = random.randint(0, w), random.randint(0, h)
        pts = [(x, y)]
        for _ in range(random.randint(2, 5)):
            if random.random() < 0.5:
                x += random.choice([-1, 1]) * random.randint(70, 200)
            else:
                y += random.choice([-1, 1]) * random.randint(70, 200)
            pts.append((x, y))
        d.line(pts, fill=(255, 255, 255, random.randint(40, 110)), width=3, joint="curve")
        for px, py in pts:
            d.ellipse([px - 5, py - 5, px + 5, py + 5], fill=(255, 255, 255, 170))
    img.alpha_composite(ov)


def bg_linen(img):
    """亚麻（浅色）：细密织纹 + 柔和斜光。"""
    ov, d = overlay(img)
    w, h = img.size
    for x in range(0, w, 6):
        d.line([(x, 0), (x, h)], fill=(120, 113, 108, 16), width=1)
    for y in range(0, h, 6):
        d.line([(0, y), (w, y)], fill=(120, 113, 108, 12), width=1)
    img.alpha_composite(ov)
    img.alpha_composite(_radial_glow(img.size, (w * 0.30, h * 0.18), w * 1.0,
                                     (255, 251, 235), 60))


BG_MOTIFS = {
    "stars": bg_stars, "mountain": bg_mountain, "city": bg_city, "bokeh": bg_bokeh,
    "waves": bg_waves, "grid": bg_grid, "diagonal": bg_diagonal, "glow": bg_glow,
    "gold": bg_gold, "mesh": bg_mesh, "aurora": bg_aurora, "sand": bg_sand,
    "canyon": bg_canyon, "forest": bg_forest, "ink": bg_ink, "geo": bg_geo,
    "lines": bg_lines, "nebula": bg_nebula, "noir": bg_noir, "ocean": bg_ocean,
    "pearl": bg_pearl, "ridge": bg_ridge, "tech": bg_tech, "linen": bg_linen,
}


def make_background(item_id, name, colors, motif, tags):
    """背景图：竖幅渐变 + motif + 暗角；JPEG 原图 + 缩略图（选择器预览）。"""
    img = vgrad((BG_W, BG_H), *colors).convert("RGBA")
    BG_MOTIFS[motif](img)
    _vignette(img)
    path = os.path.join(OUT, "backgrounds", item_id + ".jpg")
    img.convert("RGB").save(path, "JPEG", quality=87)
    thumb = img.convert("RGB").resize((BG_THUMB_W, BG_THUMB_H), Image.LANCZOS)
    tpath = os.path.join(OUT, "thumbs", "backgrounds", item_id + ".jpg")
    thumb.save(tpath, "JPEG", quality=80)
    return {"id": item_id, "name": name, "category": "background",
            "file": "backgrounds/%s.jpg" % item_id,
            "thumb": "thumbs/backgrounds/%s.jpg" % item_id,
            "width": BG_W, "height": BG_H, "tags": tags + ["背景", "全屏", motif]}


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
- **封面（covers/）、背景（backgrounds/）、LOGO（logos/）与头像（avatars/）**：
  由生成脚本程序绘制（渐变 + 几何图形/人形剪影），无第三方素材，可自由使用。

重新生成：`python3 gen_library.py`
（依赖 Pillow 与 rsvg-convert；Lucide SVG 已留存 library-src/icons/，离线可再生成）。
"""


def main():
    for sub in ("icons", "covers", "logos", "avatars", "backgrounds",
                "thumbs/covers", "thumbs/backgrounds"):
        os.makedirs(os.path.join(OUT, sub), exist_ok=True)
    fetch_lucide_license()

    items = []
    print("icons: %d" % len(ICONS))
    for lucide, name, group, tags in ICONS:
        items.append(make_icon("icon-" + lucide, name, lucide, group, tags))
    print("covers: %d" % len(COVERS))
    for cid, name, colors, motif in COVERS:
        items.append(make_cover(cid, name, colors, motif))
    print("logos: %d" % len(LOGOS))
    for lid, name, colors, mark in LOGOS:
        items.append(make_logo(lid, name, colors, mark))
    print("avatars: %d" % len(AVATARS))
    for aid, name, colors in AVATARS:
        items.append(make_avatar(aid, name, colors))
    print("backgrounds: %d" % len(BACKGROUNDS))
    for bid, name, colors, motif, tags in BACKGROUNDS:
        items.append(make_background(bid, name, colors, motif, tags))

    # 素材 id 必须唯一（id 同时是落盘文件名与采用路径）：
    # 扩充素材时若不小心写重，后面会静默覆盖前一项，故在此 fail-fast。
    seen = {}
    for it in items:
        if it["id"] in seen:
            raise RuntimeError("素材 id 重复: %s（%s / %s）" % (it["id"], seen[it["id"]], it["name"]))
        seen[it["id"]] = it["name"]

    manifest = {
        "version": 3,
        "categories": [
            {"key": "icon", "name": "图标"},
            {"key": "cover", "name": "封面"},
            {"key": "logo", "name": "LOGO"},
            {"key": "avatar", "name": "头像"},
            {"key": "background", "name": "背景"},
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

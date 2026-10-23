<template>
  <div class="stat-trend">
    <div class="trend-legend">
      <span v-for="s in seriesDef" :key="s.key" class="legend-item">
        <i class="dot" :style="{ background: s.color }" />{{ s.label }}
      </span>
      <span v-if="points.length" class="trend-range">{{ rangeText }}</span>
    </div>
    <svg
      v-if="points.length"
      class="trend-svg"
      viewBox="0 0 680 220"
      role="img"
      aria-label="按天趋势"
    >
      <!-- 横向网格线 + Y 轴刻度（最大值 / 中值 / 0） -->
      <g>
        <line
          v-for="(g, i) in gridYs"
          :key="'g' + i"
          class="grid-line"
          :x1="PAD.l"
          :x2="W - PAD.r"
          :y1="g.y"
          :y2="g.y"
        />
        <text
          v-for="(g, i) in gridYs"
          :key="'gt' + i"
          class="axis-label"
          :x="PAD.l - 8"
          :y="g.y + 4"
          text-anchor="end"
        >{{ g.label }}</text>
      </g>
      <!-- X 轴日期标签：首 / 中 / 尾（点密集时全打会糊成一团） -->
      <text
        v-for="(x, i) in xTicks"
        :key="'x' + i"
        class="axis-label"
        :x="x.x"
        :y="H - 8"
        text-anchor="middle"
      >{{ x.label }}</text>
      <!-- 访问量的面积底（只给主指标，避免三条面积互相盖） -->
      <polygon v-if="points.length > 1" class="area" :points="areaPoints" />
      <!-- 三条折线：non-scaling-stroke 保证 viewBox 缩放后线宽不跟着变 -->
      <polyline
        v-for="s in series"
        :key="s.key"
        :points="s.points"
        fill="none"
        :stroke="s.color"
        stroke-width="2"
        stroke-linejoin="round"
        stroke-linecap="round"
        vector-effect="non-scaling-stroke"
      />
      <!-- 数据点：原生 <title> 作 tooltip，零 JS 依赖 -->
      <g v-for="s in series" :key="'d' + s.key">
        <circle
          v-for="(p, i) in s.dots"
          :key="i"
          :cx="p.x"
          :cy="p.y"
          r="3"
          :fill="s.color"
        >
          <title>{{ p.date }} {{ s.label }} {{ p.value }}</title>
        </circle>
      </g>
    </svg>
    <p v-else class="trend-empty">所选时段暂无数据</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

export interface TrendPoint {
  date: string
  visit: number
  call: number
  save: number
}

const props = defineProps<{ points: TrendPoint[] }>()

// 画布几何：viewBox 坐标系，外层 width:100% 等比缩放
const W = 680
const H = 220
const PAD = { l: 44, r: 16, t: 16, b: 28 }
const plotW = W - PAD.l - PAD.r
const plotH = H - PAD.t - PAD.b

type SeriesKey = 'visit' | 'call' | 'save'
const seriesDef: { key: SeriesKey; label: string; color: string }[] = [
  { key: 'visit', label: '访问', color: '#409eff' },
  { key: 'call', label: '拨号', color: '#67c23a' },
  { key: 'save', label: '保存', color: '#e6a23c' },
]

// 纵轴上限至少为 1：全零时分母不能是 0，且「有数据但都是 0」与「无数据」
// 在图上要有区别（前者画一条贴底的线，后者走空态文案）
const maxValue = computed(() =>
  Math.max(1, ...props.points.flatMap((p) => [p.visit, p.call, p.save]))
)

// 横坐标按索引均分（不是按日期）：缺事件的日期不占位，
// 由后端只回有事件的日期、前端按序连点，避免两边各写一套日期推导。
const xOf = (i: number) =>
  props.points.length <= 1 ? PAD.l + plotW / 2 : PAD.l + (i * plotW) / (props.points.length - 1)
const yOf = (v: number) => PAD.t + plotH - (v / maxValue.value) * plotH

const series = computed(() =>
  seriesDef.map((s) => {
    const dots = props.points.map((p, i) => ({
      x: xOf(i),
      y: yOf(p[s.key] || 0),
      date: p.date,
      value: p[s.key] || 0,
    }))
    return { ...s, dots, points: dots.map((d) => `${d.x},${d.y}`).join(' ') }
  })
)

// 面积底 = 折线 + 回到底边两个角的多边形
const areaPoints = computed(() => {
  const s = series.value[0]
  if (!s || props.points.length < 2) return ''
  const base = PAD.t + plotH
  const back = props.points.map((_, i) => `${xOf(i)},${base}`).reverse().join(' ')
  return `${s.points} ${back}`
})

const gridYs = computed(() => {
  const m = maxValue.value
  return [
    { y: PAD.t, label: String(m) },
    { y: PAD.t + plotH / 2, label: String(Math.round(m / 2)) },
    { y: PAD.t + plotH, label: '0' },
  ]
})

const xTicks = computed(() => {
  const n = props.points.length
  if (n === 0) return []
  const idx = n === 1 ? [0] : n === 2 ? [0, 1] : [0, Math.floor((n - 1) / 2), n - 1]
  // 只显示月-日：x 轴空间有限，且范围已由上方文字说明
  return idx.map((i) => ({ x: xOf(i), label: props.points[i].date.slice(5) }))
})

const rangeText = computed(() => {
  const n = props.points.length
  if (!n) return ''
  return n === 1 ? props.points[0].date : `${props.points[0].date} ~ ${props.points[n - 1].date}`
})
</script>

<style scoped>
.stat-trend {
  margin: 4px 0 16px;
}
.trend-legend {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 4px;
  font-size: 13px;
  color: #606266;
}
.legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
}
.trend-range {
  margin-left: auto;
  color: #909399;
  font-size: 12px;
}
.trend-svg {
  width: 100%;
  height: auto;
  display: block;
}
.grid-line {
  stroke: #ebeef5;
  stroke-width: 1;
}
.axis-label {
  fill: #909399;
  font-size: 11px;
}
.area {
  fill: rgba(64, 158, 255, 0.08);
  stroke: none;
}
.trend-empty {
  margin: 8px 0;
  padding: 24px 0;
  text-align: center;
  color: #909399;
  font-size: 13px;
  background: #fafcff;
  border: 1px dashed #ebeef5;
  border-radius: 6px;
}
</style>

<template>
  <!-- 统一的全屏大图查看器：在原图分辨率 / 文件大小上给管理员确定性信息，
       避免"看着像是高清、实际糊图"或"图太大拖慢小程序"这类问题到上线才暴露。
       由父级 v-if 控制挂载，挂载即代表打开；关闭时 emit('close') 让父级卸载。 -->
  <Teleport to="body">
    <el-image-viewer
      :url-list="urlList"
      :initial-index="index"
      :z-index="3000"
      hide-on-click-modal
      @close="emit('close')"
      @switch="onSwitch"
    />
    <div class="img-viewer-info">
      <!-- 胶囊底：信息条位于图片区上方而非贴底，亮色图片上也能看清 -->
      <div class="ivi-pill">
        <span v-if="urlList.length > 1" class="ivi-item">{{ index + 1 }} / {{ urlList.length }}</span>
        <span class="ivi-name" :title="currentUrl">{{ name }}</span>
        <span class="ivi-item" :class="{ muted: !resolution }">{{ resolution || '分辨率未知' }}</span>
        <span class="ivi-item" :class="{ muted: !size }">{{ size || '大小未知' }}</span>
        <span v-if="loading" class="ivi-item muted">读取中…</span>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'

const props = defineProps({
  urlList: { type: Array as () => string[], required: true },
  initialIndex: { type: Number, default: 0 },
})
const emit = defineEmits(['close'])

const index = ref(props.initialIndex)
const resolution = ref('')
const size = ref('')
const loading = ref(false)

const currentUrl = computed(() => props.urlList[index.value] || '')
// blob: 说明是刚选好还没提交的本地预览，HEAD/字节数都拿不到，直接说明来源
const name = computed(() => {
  const u = currentUrl.value
  if (!u) return ''
  if (u.startsWith('blob:')) return '本地预览（未提交）'
  return decodeURIComponent(u.split('/').pop() || '').split('?')[0]
})

function fmtBytes(n: number) {
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(0) + ' KB'
  return (n / 1024 / 1024).toFixed(2) + ' MB'
}

// 分辨率靠 Image 解码取 naturalWidth/Height；体积优先 HEAD 的 Content-Length
// （不下载整图），拿不到再退回落 blob 实测。两者都失败就留空并显示"未知"。
async function loadMeta(url: string) {
  resolution.value = ''
  size.value = ''
  if (!url) return
  loading.value = true
  const img = new Image()
  img.onload = () => {
    resolution.value = `${img.naturalWidth}×${img.naturalHeight}`
  }
  img.src = url
  try {
    const head = await fetch(url, { method: 'HEAD' })
    const len = Number(head.headers.get('content-length') || 0)
    if (len > 0) {
      size.value = fmtBytes(len)
      loading.value = false
      return
    }
    throw new Error('no content-length')
  } catch {
    try {
      const blob = await (await fetch(url)).blob()
      size.value = fmtBytes(blob.size)
    } catch {
      size.value = ''
    }
  }
  loading.value = false
}

function onSwitch(i: number) {
  index.value = i
  loadMeta(props.urlList[i])
}

onMounted(() => loadMeta(currentUrl.value))
watch(() => props.urlList, () => loadMeta(currentUrl.value))
</script>

<style scoped>
/* 信息条：查看器本身的 z-index 已设为 3000，这里再高一层。
   位置放在底部工具栏「上方」——Element Plus 的 .el-image-viewer__actions 是
   bottom:30px / 高 44px（占 30~74px），故信息条取 86px，中间留 12px 间隙，
   既不压按钮也不贴视口边缘。 */
.img-viewer-info {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 86px;
  z-index: 3001;
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
}
.ivi-pill {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 7px 18px;
  border-radius: 18px;
  background: rgba(0, 0, 0, 0.45);
  color: #fff;
  font-size: 13px;
  backdrop-filter: blur(2px);
}
.ivi-name {
  max-width: 40vw;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
}
.ivi-item.muted {
  color: #c0c4cc;
}
</style>

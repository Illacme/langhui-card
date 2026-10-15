<template>
  <div class="rich-editor">
    <div class="rich-toolbar">
      <el-button size="small" :icon="VideoCamera" @click="insertWxVideo">插入视频号块</el-button>
      <span class="tip">视频号块会序列化为 &lt;wx-video channel-id="gh_xxx"&gt;，供小程序端渲染</span>
    </div>
    <div class="editor-wrap">
      <Toolbar class="editor-toolbar" :editor="editorRef" :defaultConfig="toolbarConfig" :mode="mode" />
      <Editor
        :style="{ height: height + 'px', overflowY: 'hidden' }"
        v-model="valueHtml"
        :defaultConfig="editorConfig"
        :mode="mode"
        @onCreated="handleCreated"
        @onChange="handleChange"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, shallowRef, watch, onBeforeUnmount } from 'vue'
import { Editor, Toolbar } from '@wangeditor/editor-for-vue'
import type { IDomEditor } from '@wangeditor/editor'
import '@wangeditor/editor/dist/css/style.css'
import { ElMessage } from 'element-plus'
import { VideoCamera } from '@element-plus/icons-vue'
import { registerWxVideoModule, WX_VIDEO_TYPE } from '@/editor/wxVideoModule'

// 必须在创建编辑器前注册自定义元素（全局仅一次）
registerWxVideoModule()

const props = defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: '请输入内容…' },
  height: { type: Number, default: 360 },
})
const emit = defineEmits(['update:modelValue'])

const mode = 'default'
const editorRef = shallowRef<IDomEditor>()
const valueHtml = ref(props.modelValue)

// 外部值变化（如重置/重新加载）同步进编辑器
watch(
  () => props.modelValue,
  (v) => {
    if (v !== valueHtml.value) valueHtml.value = v || ''
  }
)

const toolbarConfig = {
  // 排除自带视频上传：本项目视频用「视频号块」承载
  excludeKeys: ['group-video', 'fullScreen'],
}
const editorConfig = {
  placeholder: props.placeholder,
}

function handleCreated(editor: IDomEditor) {
  editorRef.value = editor
}

function handleChange(editor: IDomEditor) {
  emit('update:modelValue', editor.getHtml())
}

// 插入视频号块：弹出输入 channel-id
function insertWxVideo() {
  const editor = editorRef.value
  if (!editor) return
  editor.restoreSelection()
  const input = window.prompt('请输入视频号 channel-id（形如 gh_xxxx）', 'gh_')
  const channelId = (input || '').trim()
  if (!channelId) {
    if (input !== null) ElMessage.warning('未输入 channel-id')
    return
  }
  editor.insertNode({
    type: WX_VIDEO_TYPE,
    channelId,
    children: [{ text: '' }],
  })
  editor.focus()
}

onBeforeUnmount(() => {
  editorRef.value?.destroy()
})
</script>

<style scoped>
.rich-editor {
  border: 1px solid #dcdfe6;
  border-radius: 4px;
  overflow: hidden;
}
.rich-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 8px;
  background: #f5f7fa;
  border-bottom: 1px solid #dcdfe6;
}
.rich-toolbar .tip {
  color: #909399;
  font-size: 12px;
}
.editor-wrap :deep(.w-e-toolbar) {
  border-bottom: 1px solid #dcdfe6;
}
/* 编辑区：视频号块占位样式 */
.rich-editor :deep(.wx-video-block) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 4px 0;
  padding: 6px 10px;
  background: #f0f9eb;
  border: 1px dashed #67c23a;
  border-radius: 4px;
  color: #67c23a;
  font-size: 13px;
}
</style>

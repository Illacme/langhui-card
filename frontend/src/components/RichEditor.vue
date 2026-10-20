<template>
  <div class="rich-editor">
    <div class="rich-toolbar">
      <el-button size="small" :icon="VideoCamera" @click="openVideoDialog()">插入视频号块</el-button>
      <span class="tip">
        视频号块序列化为 &lt;wx-video feed-id="…" feed-token="…" channel-id="sph_…"&gt;；
        <b>点击已有块可重新编辑</b>；块在末尾时会自动留一个空段落，方便在它后面继续输入
      </span>
    </div>
    <!-- 编辑区点击委托：点到视频号块就地打开编辑弹窗，不用删除后重加 -->
    <div class="editor-wrap" @click="onEditorClick">
      <Toolbar class="editor-toolbar" :editor="editorRef" :defaultConfig="toolbarConfig" :mode="mode" />
      <Editor
        :style="{ height: height + 'px', overflowY: 'hidden' }"
        :defaultHtml="valueHtml"
        :defaultConfig="editorConfig"
        :mode="mode"
        @onCreated="handleCreated"
        @onChange="handleChange"
      />
    </div>

    <el-dialog
      v-model="videoDialogVisible"
      :title="editingMode ? '编辑视频号块' : '插入视频号块'"
      width="520px"
      append-to-body
    >
      <el-form :model="videoForm" label-width="110px">
        <el-form-item label="feedId">
          <el-input v-model="videoForm.feedId" placeholder="视频号助手 → 动态管理 → 复制 feedId" />
          <div class="field-hint">
            决定播哪一条视频。<b>同主体</b>填这个即可（可自动播放）。与 feed-token 二选一。
          </div>
        </el-form-item>
        <el-form-item label="feed-token">
          <el-input v-model="videoForm.feedToken" placeholder="跨主体才填：MP 平台开权限后移动端复制" />
          <div class="field-hint">
            <b>跨主体才需要</b>：视频号与我们小程序不同主体时 feed-id 无效，必须改用它。
            代价是<b>强制不能自动播放</b>（用户需点一下）。
            <span v-if="videoForm.feedId.trim()" class="field-warn">已填 feedId，本项不会生效。</span>
          </div>
        </el-form-item>
        <el-form-item label="视频号 ID">
          <el-input v-model="videoForm.channelId" placeholder="sph 开头，可留空" />
          <div class="field-hint">
            选填，仅作留档。视频号 ID 是 <b>sph</b> 开头（<code>gh_</code> 是公众号 ID）。
            只填它而不填 feedId 时，小程序端只能展示视频号主页。
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="videoDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="confirmVideo">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { ref, reactive, shallowRef, watch, onBeforeUnmount } from 'vue'
import { Editor, Toolbar } from '@wangeditor/editor-for-vue'
// SlateTransforms 由 @wangeditor/editor 直接再导出（它内部依赖 slate），
// 不必为了「就地更新节点」再单独引入 slate 依赖。
import { SlateEditor, SlateTransforms, type IDomEditor } from '@wangeditor/editor'
import '@wangeditor/editor/dist/css/style.css'
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
// 只在「创建编辑器」时读一次的初始 HTML（见下方 watch 的说明）
const valueHtml = ref(props.modelValue)

// 程序灌内容期间（清空 → 逐段插入）的中间态不要回传给父组件：
// 否则「清空」那一瞬间的空 HTML 会把父组件的 v-model 冲掉，用户点保存就存了个空。
let programmatic = false

// 外部值变化（切到另一条记录编辑 / 弹窗关闭后重置）同步进编辑器。
//
// 两层坑，缺一不可：
//
// 1）不能只靠 editor-for-vue 的 v-model：它用自己缓存的 curValue 判等，而 curValue
//    只在 onChange 里更新。这里以编辑器的真实内容（editor.getHtml()）为唯一准绳 ——
//    编辑器还没创建时只更新 valueHtml（创建时会被当作初始 HTML）；已创建则内容不同
//    才灌，用户自己输入时两者相等，不会打断光标。
//
// 2）灌之前必须先把选区「归位」。wangEditor 的 setHtml() 内部会先 focus()，而 focus()
//    会把它在 EDITOR_TO_SELECTION 里记住的选区恢复回来。弹窗关闭时内容被清空，但那份
//    记住的选区仍指向旧内容的第 N 个节点；再次打开时文档只剩一个空段落，它就成了
//    悬空引用 —— Transforms.select 不校验路径合法性，紧接着的 clear() 里
//    Editor.above → Editor.void 会直接抛
//    「Cannot find a descendant at path [N]」，setHtml 中断，编辑器保持空白。
//    这就是**「保存后再次打开编辑页，富文本框是空的，得再关一次才恢复」**的根因。
//    对策：先把选区挪到「任何文档下都成立」的第一个文本节点，再等一个微任务让
//    wangEditor 把这份合法选区记住（它在 onChange 里记，而 onChange 是微任务里触发的）。
async function pushValueIntoEditor(next: string) {
  const editor = editorRef.value
  if (!editor) return
  programmatic = true
  try {
    SlateTransforms.select(editor, { path: [0, 0], offset: 0 } as any)
  } catch {
    /* 文档结构异常时忽略，后面 setHtml 会重建 */
  }
  // 让 slate 的 onChange 落地：刷新 wangEditor 记住的选区（微任务 FIFO，排在上面那次之后）
  await Promise.resolve()
  if (editorRef.value !== editor) {
    programmatic = false // 组件已卸载 / 编辑器已重建，别把标志位留在 true 上
    return
  }
  try {
    editor.setHtml(next)
  } finally {
    programmatic = false
  }
  // normalize（如末尾视频块的光标落点段落被剥掉）可能让最终内容与期望值略有出入，
  // 这里对齐一次；相等则不发，避免多余的一次往返。
  const finalHtml = editor.getHtml()
  if (finalHtml !== next) emit('update:modelValue', finalHtml)
}

watch(
  () => props.modelValue,
  (v) => {
    const next = v || ''
    valueHtml.value = next
    const editor = editorRef.value
    if (!editor) return
    if (editor.getHtml() === next) return
    void pushValueIntoEditor(next)
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
  // 程序灌内容期间的中间态不回传（详见 pushValueIntoEditor 的注释）
  if (programmatic) return
  emit('update:modelValue', editor.getHtml())
}

// ===== 视频号块：新增 / 编辑 =====
const videoDialogVisible = ref(false)
const videoForm = reactive({ feedId: '', feedToken: '', channelId: '' })
// true = 编辑已有块（按点击时的原值定位）；false = 新增
const editingMode = ref(false)
// 点击时的原值三元组：确认时用它全文匹配定位目标节点
const editingOld = reactive({ feedId: '', feedToken: '', channelId: '' })

function openVideoDialog(feedId = '', feedToken = '', channelId = '', editing = false) {
  videoForm.feedId = feedId
  videoForm.feedToken = feedToken
  videoForm.channelId = channelId
  editingMode.value = editing
  editingOld.feedId = feedId
  editingOld.feedToken = feedToken
  editingOld.channelId = channelId
  videoDialogVisible.value = true
}

// 编辑区点击委托：命中视频号块就带出现值打开弹窗。
// 不用 renderElem 的 on.click——wangEditor 注册的 snabbdom 模块只保证 props / dataset，
// 事件绑定不在此列，走 DOM 委托最稳。
function onEditorClick(e: MouseEvent) {
  const t = e.target
  if (!(t instanceof HTMLElement)) return
  const block = t.closest('.wx-video-block')
  if (!block) return
  openVideoDialog(
    block.getAttribute('data-feed-id') || '',
    block.getAttribute('data-feed-token') || '',
    block.getAttribute('data-channel-id') || '',
    true
  )
}

// 按三属性全文匹配定位视频号块。
// 不能用节点 id：wangEditor 的 slate 节点没有 id 属性（getElemsByType 返回的 id 是
// findKey 动态包装的），elem.id 恒为 undefined，靠它定位会静默失败误走新增分支。
function findVideoPath(
  editor: IDomEditor,
  feedId: string,
  feedToken: string,
  channelId: string
): number[] | null {
  const entries = SlateEditor.nodes(editor, {
    at: [],
    match: (n: any) =>
      n.type === WX_VIDEO_TYPE &&
      (n.feedId || '') === feedId &&
      (n.feedToken || '') === feedToken &&
      (n.channelId || '') === channelId,
  } as any)
  for (const [, path] of entries) return path as number[]
  return null
}

// 视频号块是 void 块，插入后光标停在块上，用户没法直接在它后面继续打字。
// 把光标移到块后面那个节点（末尾时 wxVideoModule 的 normalizeNode 已补好空段落）。
function moveCaretAfterVideo(
  editor: IDomEditor,
  feedId: string,
  feedToken: string,
  channelId: string
) {
  const path = findVideoPath(editor, feedId, feedToken, channelId)
  if (!path) return
  const nextIndex = path[0] + 1
  if (nextIndex >= (editor.children || []).length) return
  try {
    SlateTransforms.select(editor, SlateEditor.start(editor, [nextIndex]))
  } catch {
    /* 极端情况下选区不可达，忽略即可，不影响内容 */
  }
}

async function confirmVideo() {
  const feedId = videoForm.feedId.trim()
  const feedToken = videoForm.feedToken.trim()
  const channelId = videoForm.channelId.trim()
  // feed-id（同主体）与 feed-token（跨主体）至少要有一个，否则播不出任何东西
  if (!feedId && !feedToken) {
    ElMessage.warning('请填写 feedId（同主体）或 feed-token（跨主体）')
    return
  }
  const editor = editorRef.value
  if (!editor) return

  const node = {
    type: WX_VIDEO_TYPE,
    feedId,
    feedToken,
    channelId,
    children: [{ text: '' }],
  } as any

  if (editingMode.value) {
    // 就地更新：先按「点击时的原值」全文搜索拿到目标节点的确切 path，再带 at 更新。
    //
    // 两个都不能省：
    //  1. 不按原值匹配、而是匹配节点 id —— wangEditor 的 slate 节点没有 id 属性，必然落空；
    //  2. setNodes 不传 at —— 会缺省落到 editor.selection 上，slate 在 range 边界拆分节点
    //     并把视频号属性带给拆出的新节点，表现为「保存后凭空多出一个块」。
    // 找不到目标（弹窗打开期间内容被并发改动等极端情况）退化为插入，至少不丢用户输入。
    const targetPath = findVideoPath(
      editor,
      editingOld.feedId,
      editingOld.feedToken,
      editingOld.channelId
    )
    if (targetPath) {
      SlateTransforms.setNodes(editor, { feedId, feedToken, channelId } as any, {
        at: targetPath,
      } as any)
    } else {
      editor.insertNode(node)
    }
  } else {
    editor.restoreSelection()
    editor.insertNode(node)
  }

  // 无论新增还是编辑，都把光标放到块之后：末尾场景以前点不到块后面，现在能直接续写。
  moveCaretAfterVideo(editor, feedId, feedToken, channelId)

  videoDialogVisible.value = false
  editor.updateView?.()
  // void 元素改属性不一定触发 onChange，这里手动同步一次避免外部拿到旧 HTML
  emit('update:modelValue', editor.getHtml())
  // 等一个微任务再 focus：wangEditor 的 focus() 会「恢复上次记住的选区」，而它是在
  // onChange（微任务）里记的 —— 不等这一拍，focus 会把刚定位到视频块后面的光标
  // 又挪回旧位置（末尾场景就又变成「光标停在块上、打不了字」）。
  await Promise.resolve()
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
  cursor: pointer;
}
.rich-editor :deep(.wx-video-block:hover) {
  background: #e8f6df;
  border-style: solid;
}
.field-hint {
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.6;
  color: #909399;
}
.field-warn {
  color: #e6a23c;
}
</style>

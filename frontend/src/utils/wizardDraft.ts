// 配置向导 / 系统设置草稿的本地暂存：刷新页面、误关标签页后回来接着改，
// 而不是对着一批空输入框从头填。保存成功或登出后清除。
//
// 单独成模块而不是把 key 写在 Settings.vue 里：auth store 登出时也要清同一份，
// 两处各写一个字符串常量，迟早改一处漏一处。

const WIZARD_DRAFT_KEY = 'settings_wizard_draft'

export interface StoredDraft {
  draft: Record<string, string>
  cleared: Record<string, boolean>
}

export function saveWizardDraft(draft: Record<string, string>, cleared: Record<string, boolean>) {
  try {
    const payload = { v: 1, draft: { ...draft }, cleared: { ...cleared } }
    localStorage.setItem(WIZARD_DRAFT_KEY, JSON.stringify(payload))
  } catch {
    // 隐私模式 / 配额满：暂存失败只影响「刷新不丢」，不影响正常使用
  }
}

export function loadWizardDraft(): StoredDraft | null {
  try {
    const raw = localStorage.getItem(WIZARD_DRAFT_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw)
    // 结构不对（旧版本 / 被改坏）按没有暂存处理，不让页面带着脏数据启动
    if (!parsed || typeof parsed !== 'object' || !parsed.draft || typeof parsed.draft !== 'object') {
      return null
    }
    return {
      draft: parsed.draft as Record<string, string>,
      cleared:
        parsed.cleared && typeof parsed.cleared === 'object'
          ? (parsed.cleared as Record<string, boolean>)
          : {},
    }
  } catch {
    return null
  }
}

export function clearWizardDraft() {
  try {
    localStorage.removeItem(WIZARD_DRAFT_KEY)
  } catch {
    // 清不掉的最坏结果是下次恢复一份旧草稿，用户改回去或重置即可
  }
}

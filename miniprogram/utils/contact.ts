// 保存名片到手机通讯录（员工名片区「保存名片」与底部「存入通讯录」共用）
// 座机与名片展示同一套回退规则：优先本人 landline，为空用公司总机 tel。
export function saveCard(
  emp: { name?: string; title?: string; phone?: string; landline?: string; email?: string },
  comp: { fullName?: string; address?: string; tel?: string }
): void {
  const landline = emp.landline || comp.tel || ''
  wx.showModal({
    title: '保存名片',
    content: '将把该联系人存入手机通讯录，是否继续？',
    success: (r) => {
      if (!r.confirm) return
      wx.addPhoneContact({
        firstName: emp.name || '',
        title: emp.title || '',
        mobilePhoneNumber: emp.phone || '',
        email: emp.email || '',
        organization: comp.fullName || '',
        addressStreet: comp.address || '',
        workPhoneNumber: landline,
        success: () => wx.showToast({ title: '已保存' }),
        fail: () => wx.showToast({ title: '已取消', icon: 'none' }),
      })
    },
  })
}

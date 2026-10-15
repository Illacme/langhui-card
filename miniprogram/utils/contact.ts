// 保存名片到手机通讯录（员工名片区「保存名片」与底部「存入通讯录」共用）
export function saveCard(
  emp: { name?: string; title?: string; phone?: string; email?: string },
  comp: { fullName?: string; address?: string; tel?: string }
): void {
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
        workPhoneNumber: comp.tel || '',
        success: () => wx.showToast({ title: '已保存' }),
        fail: () => wx.showToast({ title: '已取消', icon: 'none' }),
      })
    },
  })
}

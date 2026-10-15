import { saveCard } from '../../utils/contact'
import { request } from '../../utils/request'

Component({
  options: { addGlobalClass: true },
  properties: {
    employee: Object,
    company: Object,
    staffId: Number,
  },
  methods: {
    onCall() {
      const phone = this.data.employee.phone
      if (!phone) return
      wx.makePhoneCall({ phoneNumber: phone })
      request('POST', '/api/stat/track', { staffId: this.data.staffId, eventType: 'call' })
    },
    onSave() {
      saveCard(this.data.employee, this.data.company)
      request('POST', '/api/stat/track', { staffId: this.data.staffId, eventType: 'save' })
    },
  },
})

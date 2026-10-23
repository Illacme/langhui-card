import { saveCard } from '../../utils/contact'
import { track } from '../../utils/track'

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
      track('call', this.data.staffId)
    },
    onSave() {
      saveCard(this.data.employee, this.data.company)
      track('save', this.data.staffId)
    },
  },
})

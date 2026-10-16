import { saveCard } from '../../utils/contact'

Component({
  options: { addGlobalClass: true },
  properties: {
    employee: Object,
    company: Object,
  },
  methods: {
    onSave() {
      saveCard(this.data.employee, this.data.company)
    },
    // 「案例进入」：只上报意图，具体行为（滚动业绩区 / 跳第一个分类 / 提示）由页面决定
    onEnterCase() {
      this.triggerEvent('entercase')
    },
  },
})

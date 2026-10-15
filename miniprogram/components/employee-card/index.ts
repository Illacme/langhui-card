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
  },
})

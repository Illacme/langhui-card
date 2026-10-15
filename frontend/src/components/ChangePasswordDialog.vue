<template>
  <el-dialog v-model="visible" title="修改密码" width="420px" @closed="onClosed">
    <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
      <el-form-item label="原密码" prop="old">
        <el-input v-model="form.old" type="password" show-password placeholder="请输入当前密码" />
      </el-form-item>
      <el-form-item label="新密码" prop="new">
        <el-input v-model="form.new" type="password" show-password placeholder="请输入新密码" />
      </el-form-item>
      <el-form-item label="确认新密码" prop="confirm">
        <el-input v-model="form.confirm" type="password" show-password placeholder="再次输入新密码" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="loading" @click="submit">确定</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { changeAdminPassword } from '@/api/auth'

const visible = defineModel<boolean>({ default: false })

const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive({ old: '', new: '', confirm: '' })

const validateConfirm = (rule: any, value: string, cb: any) => {
  if (value !== form.new) cb(new Error('两次输入不一致'))
  else cb()
}

const rules: FormRules = {
  old: [{ required: true, message: '请输入原密码', trigger: 'blur' }],
  new: [{ required: true, message: '请输入新密码', trigger: 'blur' }],
  confirm: [{ validator: validateConfirm, trigger: 'blur' }],
}

function submit() {
  formRef.value?.validate(async (ok) => {
    if (!ok) return
    loading.value = true
    try {
      await changeAdminPassword(form.old, form.new)
      ElMessage.success('密码已修改，请牢记新密码')
      visible.value = false
    } catch {
      // 拦截器已提示（如「原密码错误」）
    } finally {
      loading.value = false
    }
  })
}

function onClosed() {
  form.old = ''
  form.new = ''
  form.confirm = ''
  formRef.value?.clearValidate()
}
</script>

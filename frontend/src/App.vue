<template>
  <!-- Element Plus 的文案默认是英文：不设 locale 时分页器会渲染成「Total 12」、
       「10/page」这类英文串。用 ConfigProvider 全局注入简体中文语言包
       （el-config-provider 只 renderSlot 不产生包裹元素，不影响现有布局）。
       分页相关文案随之变成「共 12 条」「10 条/页」。 -->
  <el-config-provider :locale="locale">
    <router-view />
  </el-config-provider>
</template>

<script setup lang="ts">
import zhCn from 'element-plus/es/locale/lang/zh-cn'

const locale = zhCn
</script>

<style>
html,
body,
#app {
  height: 100%;
  margin: 0;
  padding: 0;
}
body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue',
    Arial, 'PingFang SC', 'Microsoft YaHei', sans-serif;
  background: #f5f7fa;
}
</style>

<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{ error?: string }>()
const emit = defineEmits<{ login: [password: string] }>()
const password = ref('')
const localError = ref('')
const loading = ref(false)

async function submit() {
  if (!password.value.trim()) {
    localError.value = '请输入密码'
    return
  }
  localError.value = ''
  loading.value = true
  emit('login', password.value.trim())
  // loading will be reset by parent re-render or error prop change
  setTimeout(() => { loading.value = false }, 3000)
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center">
    <div class="bg-slate-800 rounded-lg p-8 w-80 shadow-xl">
      <h1 class="text-xl font-bold text-center mb-6">CC2API</h1>
      <form @submit.prevent="submit">
        <input
          v-model="password"
          type="password"
          placeholder="管理员密码"
          class="w-full px-3 py-2 bg-slate-700 rounded border border-slate-600 text-white placeholder-slate-400 focus:outline-none focus:border-blue-500 mb-4"
        />
        <p v-if="localError || props.error" class="text-red-400 text-sm mb-2">{{ localError || props.error }}</p>
        <button
          type="submit"
          class="w-full py-2 bg-blue-600 hover:bg-blue-700 rounded text-white font-medium transition"
        >
          登录
        </button>
      </form>
    </div>
  </div>
</template>

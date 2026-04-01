<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { setAuth, api } from './api'
import Login from './components/Login.vue'
import Dashboard from './components/Dashboard.vue'

const loggedIn = ref(false)
const loginError = ref('')

async function handleLogin(password: string) {
  setAuth(password)
  try {
    await api.getDashboard()
    loggedIn.value = true
    loginError.value = ''
    localStorage.setItem('cc2api_auth', password)
  } catch {
    loginError.value = '密码错误'
    setAuth('')
  }
}

onMounted(async () => {
  const saved = localStorage.getItem('cc2api_auth')
  if (saved) {
    await handleLogin(saved)
  }
})

function logout() {
  loggedIn.value = false
  localStorage.removeItem('cc2api_auth')
}
</script>

<template>
  <Login v-if="!loggedIn" :error="loginError" @login="handleLogin" />
  <Dashboard v-else @logout="logout" />
</template>

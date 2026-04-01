<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api, type Dashboard as DashboardData } from '../api'
import Accounts from './Accounts.vue'
import Usage from './Usage.vue'

const emit = defineEmits<{ logout: [] }>()
const tab = ref<'accounts' | 'usage'>('accounts')
const dashboard = ref<DashboardData | null>(null)

async function loadDashboard() {
  try {
    dashboard.value = await api.getDashboard()
  } catch {
    // ignore - don't logout on transient errors
  }
}

onMounted(loadDashboard)
</script>

<template>
  <div class="min-h-screen">
    <!-- Header -->
    <header class="bg-slate-800 border-b border-slate-700 px-6 py-3 flex items-center justify-between">
      <div class="flex items-center gap-6">
        <h1 class="text-lg font-bold">CC2API</h1>
        <nav class="flex gap-1">
          <button
            v-for="t in (['accounts', 'usage'] as const)"
            :key="t"
            @click="tab = t"
            class="px-3 py-1 rounded text-sm transition"
            :class="tab === t ? 'bg-blue-600 text-white' : 'text-slate-400 hover:text-white'"
          >
            {{ t === 'accounts' ? '账号管理' : '用量统计' }}
          </button>
        </nav>
      </div>
      <div class="flex items-center gap-4">
        <div v-if="dashboard" class="flex gap-4 text-sm text-slate-400">
          <span>账号: <b class="text-green-400">{{ dashboard.accounts.active }}</b>/{{ dashboard.accounts.total }}</span>
          <span>24h请求: <b class="text-blue-400">{{ dashboard.usage_24h.requests }}</b></span>
        </div>
        <button @click="$emit('logout')" class="text-sm text-slate-500 hover:text-white transition">退出</button>
      </div>
    </header>

    <!-- Content -->
    <main class="p-6">
      <Accounts v-if="tab === 'accounts'" @refresh="loadDashboard" />
      <Usage v-else />
    </main>
  </div>
</template>

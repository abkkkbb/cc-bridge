<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { api, type UsageStats } from '../api'

const hours = ref(24)
const stats = ref<UsageStats[]>([])

async function load() {
  try {
    stats.value = await api.getUsage(hours.value) ?? []
  } catch {
    stats.value = []
  }
}

onMounted(load)
watch(hours, load)

function formatNum(n: number) {
  return n.toLocaleString()
}
</script>

<template>
  <div>
    <div class="flex justify-between items-center mb-4">
      <h2 class="text-lg font-semibold">用量统计</h2>
      <select v-model="hours" class="bg-slate-700 text-white text-sm rounded px-3 py-1 border border-slate-600">
        <option :value="1">最近 1 小时</option>
        <option :value="6">最近 6 小时</option>
        <option :value="24">最近 24 小时</option>
        <option :value="168">最近 7 天</option>
        <option :value="720">最近 30 天</option>
      </select>
    </div>

    <div class="bg-slate-800 rounded-lg overflow-hidden">
      <table class="w-full text-sm">
        <thead>
          <tr class="text-slate-400 text-left">
            <th class="px-4 py-2">账号</th>
            <th class="px-4 py-2 text-right">请求数</th>
            <th class="px-4 py-2 text-right">输入 Token</th>
            <th class="px-4 py-2 text-right">输出 Token</th>
            <th class="px-4 py-2 text-right">缓存读取</th>
            <th class="px-4 py-2 text-right">缓存创建</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="s in stats" :key="s.account_id" class="border-t border-slate-700">
            <td class="px-4 py-2">{{ s.account_name || `#${s.account_id}` }}</td>
            <td class="px-4 py-2 text-right text-blue-400">{{ formatNum(s.total_requests) }}</td>
            <td class="px-4 py-2 text-right">{{ formatNum(s.total_input_tokens) }}</td>
            <td class="px-4 py-2 text-right">{{ formatNum(s.total_output_tokens) }}</td>
            <td class="px-4 py-2 text-right text-green-400">{{ formatNum(s.total_cache_read) }}</td>
            <td class="px-4 py-2 text-right text-yellow-400">{{ formatNum(s.total_cache_creation) }}</td>
          </tr>
          <tr v-if="stats.length === 0">
            <td colspan="6" class="px-4 py-8 text-center text-slate-500">暂无数据</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

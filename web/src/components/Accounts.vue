<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api, type Account } from '../api'

const emit = defineEmits<{ refresh: [] }>()

const accounts = ref<Account[]>([])
const showForm = ref(false)
const editing = ref<Account | null>(null)
const form = ref({ name: '', email: '', token: '', proxy_url: '', concurrency: 3, priority: 50 })
const testing = ref<number | null>(null)
const testResult = ref<{ status: string; message?: string } | null>(null)

async function load() {
  try {
    accounts.value = await api.listAccounts() ?? []
  } catch {
    accounts.value = []
  }
}

onMounted(load)

function openCreate() {
  editing.value = null
  form.value = { name: '', email: '', token: '', proxy_url: '', concurrency: 3, priority: 50 }
  showForm.value = true
}

function openEdit(a: Account) {
  editing.value = a
  form.value = {
    name: a.name,
    email: a.email,
    token: '',
    proxy_url: a.proxy_url,
    concurrency: a.concurrency,
    priority: a.priority,
  }
  showForm.value = true
}

async function save() {
  if (editing.value) {
    const updates: Record<string, unknown> = {}
    if (form.value.name) updates.name = form.value.name
    if (form.value.email) updates.email = form.value.email
    if (form.value.token) updates.token = form.value.token
    updates.proxy_url = form.value.proxy_url
    updates.concurrency = form.value.concurrency
    updates.priority = form.value.priority
    await api.updateAccount(editing.value.id, updates)
  } else {
    await api.createAccount(form.value)
  }
  showForm.value = false
  await load()
  emit('refresh')
}

async function remove(id: number) {
  if (!confirm('确认删除此账号？')) return
  await api.deleteAccount(id)
  await load()
  emit('refresh')
}

async function test(id: number) {
  testing.value = id
  testResult.value = null
  testResult.value = await api.testAccount(id)
  setTimeout(() => { testing.value = null; testResult.value = null }, 3000)
}

function statusColor(s: string) {
  return s === 'active' ? 'text-green-400' : s === 'error' ? 'text-red-400' : 'text-slate-500'
}

function maskToken(t: string) {
  if (t.length <= 16) return t
  return t.slice(0, 8) + '...' + t.slice(-8)
}
</script>

<template>
  <div>
    <div class="flex justify-between items-center mb-4">
      <h2 class="text-lg font-semibold">账号管理</h2>
      <button @click="openCreate" class="px-4 py-1.5 bg-blue-600 hover:bg-blue-700 rounded text-sm text-white transition">
        添加账号
      </button>
    </div>

    <!-- Account Table -->
    <div class="bg-slate-800 rounded-lg overflow-hidden">
      <table class="w-full text-sm">
        <thead class="bg-slate-750">
          <tr class="text-slate-400 text-left">
            <th class="px-4 py-2">名称</th>
            <th class="px-4 py-2">邮箱</th>
            <th class="px-4 py-2">状态</th>
            <th class="px-4 py-2">Token</th>
            <th class="px-4 py-2">代理</th>
            <th class="px-4 py-2">并发</th>
            <th class="px-4 py-2">优先级</th>
            <th class="px-4 py-2">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="a in accounts" :key="a.id" class="border-t border-slate-700 hover:bg-slate-750">
            <td class="px-4 py-2">{{ a.name || '-' }}</td>
            <td class="px-4 py-2 text-slate-400">{{ a.email }}</td>
            <td class="px-4 py-2">
              <span :class="statusColor(a.status)" class="font-medium">{{ a.status }}</span>
            </td>
            <td class="px-4 py-2 font-mono text-xs text-slate-500">{{ maskToken(a.token) }}</td>
            <td class="px-4 py-2 text-slate-500 text-xs">{{ a.proxy_url || '直连' }}</td>
            <td class="px-4 py-2 text-center">{{ a.concurrency }}</td>
            <td class="px-4 py-2 text-center">{{ a.priority }}</td>
            <td class="px-4 py-2">
              <div class="flex gap-2">
                <button @click="openEdit(a)" class="text-blue-400 hover:text-blue-300 text-xs">编辑</button>
                <button @click="test(a.id)" class="text-yellow-400 hover:text-yellow-300 text-xs" :disabled="testing === a.id">
                  {{ testing === a.id ? '测试中...' : '测试' }}
                </button>
                <button @click="remove(a.id)" class="text-red-400 hover:text-red-300 text-xs">删除</button>
              </div>
              <div v-if="testing === a.id && testResult" class="text-xs mt-1" :class="testResult.status === 'ok' ? 'text-green-400' : 'text-red-400'">
                {{ testResult.status === 'ok' ? '连接正常' : testResult.message }}
              </div>
            </td>
          </tr>
          <tr v-if="accounts.length === 0">
            <td colspan="8" class="px-4 py-8 text-center text-slate-500">暂无账号，点击"添加账号"开始</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Add/Edit Modal -->
    <div v-if="showForm" class="fixed inset-0 bg-black/50 flex items-center justify-center z-50" @click.self="showForm = false">
      <div class="bg-slate-800 rounded-lg p-6 w-96 shadow-xl">
        <h3 class="text-lg font-semibold mb-4">{{ editing ? '编辑账号' : '添加账号' }}</h3>
        <form @submit.prevent="save" class="space-y-3">
          <div>
            <label class="text-sm text-slate-400">备注名（选填）</label>
            <input v-model="form.name" class="w-full px-3 py-1.5 bg-slate-700 rounded border border-slate-600 text-white text-sm focus:outline-none focus:border-blue-500" />
          </div>
          <div>
            <label class="text-sm text-slate-400">邮箱 <span class="text-red-400">*</span></label>
            <input v-model="form.email" required class="w-full px-3 py-1.5 bg-slate-700 rounded border border-slate-600 text-white text-sm focus:outline-none focus:border-blue-500" />
          </div>
          <div>
            <label class="text-sm text-slate-400">OAuth Token (sk-ant-oat01-...) <span class="text-red-400">*</span></label>
            <textarea v-model="form.token" :required="!editing" rows="3" class="w-full px-3 py-1.5 bg-slate-700 rounded border border-slate-600 text-white text-sm font-mono focus:outline-none focus:border-blue-500" :placeholder="editing ? '留空保持不变' : ''" />
          </div>
          <div>
            <label class="text-sm text-slate-400">代理地址（选填）</label>
            <input v-model="form.proxy_url" placeholder="http:// 或 socks5://" class="w-full px-3 py-1.5 bg-slate-700 rounded border border-slate-600 text-white text-sm focus:outline-none focus:border-blue-500" />
          </div>
          <div class="flex gap-3">
            <div class="flex-1">
              <label class="text-sm text-slate-400">并发数</label>
              <input v-model.number="form.concurrency" type="number" min="1" class="w-full px-3 py-1.5 bg-slate-700 rounded border border-slate-600 text-white text-sm focus:outline-none focus:border-blue-500" />
            </div>
            <div class="flex-1">
              <label class="text-sm text-slate-400">优先级</label>
              <input v-model.number="form.priority" type="number" min="1" class="w-full px-3 py-1.5 bg-slate-700 rounded border border-slate-600 text-white text-sm focus:outline-none focus:border-blue-500" />
            </div>
          </div>
          <div class="flex gap-2 pt-2">
            <button type="submit" class="flex-1 py-1.5 bg-blue-600 hover:bg-blue-700 rounded text-white text-sm transition">保存</button>
            <button type="button" @click="showForm = false" class="flex-1 py-1.5 bg-slate-600 hover:bg-slate-500 rounded text-white text-sm transition">取消</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

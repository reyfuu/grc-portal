<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api.js'
import { store } from '../store.js'

const router = useRouter()
const email = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  busy.value = true
  try {
    store.user = await api.post('/login', { email: email.value, password: password.value })
    router.push('/')
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center p-4"
       style="background:linear-gradient(180deg,#eef2ff 0%,#f6f7f9 60%)">
    <div class="w-full max-w-sm">
      <div class="flex flex-col items-center mb-6">
        <div class="w-14 h-14 rounded-2xl bg-brand flex items-center justify-center text-3xl shadow-md mb-3">🛡️</div>
        <h1 class="text-xl font-bold text-ink" style="margin:0">GRC Portal</h1>
        <p class="text-sm text-muted mt-1">Governance · Risk · Compliance</p>
      </div>
      <form @submit.prevent="submit"
            class="bg-[var(--panel)] border border-[var(--border)] rounded-2xl shadow-sm p-6 flex flex-col gap-4">
        <div class="text-base font-semibold text-ink">Masuk ke akun</div>
        <label class="block" style="margin:0">
          <span class="text-xs font-medium text-muted">Email</span>
          <input v-model="email" type="email" autocomplete="username" required placeholder="admin@grc.local" class="mt-1" />
        </label>
        <label class="block" style="margin:0">
          <span class="text-xs font-medium text-muted">Password</span>
          <input v-model="password" type="password" autocomplete="current-password" required placeholder="••••••••" class="mt-1" />
        </label>
        <div v-if="error" class="err" style="margin:0" role="alert">{{ error }}</div>
        <button class="primary w-full" :disabled="busy">{{ busy ? 'Memproses…' : 'Login' }}</button>
      </form>
      <p class="text-center text-xs text-muted mt-4">Portal internal · akses terbatas</p>
    </div>
  </div>
</template>

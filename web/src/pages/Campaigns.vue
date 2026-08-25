<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api.js'
import { isAdmin } from '../store.js'

const router = useRouter()
const campaigns = ref([])
const apps = ref([])
const error = ref('')
const form = ref({ name: '', period_start: '', period_end: '', application_ids: [] })

async function load() {
  try {
    ;[campaigns.value, apps.value] = await Promise.all([api.get('/campaigns'), api.get('/applications')])
  } catch (e) {
    error.value = e.message
  }
}

async function create() {
  error.value = ''
  if (!form.value.application_ids.length) { error.value = 'Pilih minimal satu aplikasi'; return }
  try {
    const res = await api.post('/campaigns', {
      ...form.value,
      application_ids: form.value.application_ids.map(Number),
    })
    router.push(`/campaigns/${res.id}`)
  } catch (e) {
    error.value = e.message
  }
}

onMounted(load)
</script>

<template>
  <div class="container">
    <h1>Campaigns</h1>
    <p class="muted">Periode review akses. Aktifkan untuk snapshot akses jadi item yang harus di-review.</p>
    <div v-if="error" class="err">{{ error }}</div>

    <div class="panel" v-if="isAdmin()">
      <h2 style="margin-top:0">Buat campaign</h2>
      <form @submit.prevent="create">
        <label>Nama *
          <input v-model="form.name" placeholder="UAR Q3 2026" required />
        </label>
        <div class="row">
          <div><label>Periode mulai <input v-model="form.period_start" type="date" /></label></div>
          <div><label>Periode selesai <input v-model="form.period_end" type="date" /></label></div>
        </div>
        <div style="font-size:13px; color:var(--muted); margin:8px 0 2px">Aplikasi dalam scope *</div>
        <div style="display:flex; flex-wrap:wrap; gap:6px 18px; padding:4px 0">
          <label v-for="a in apps" :key="a.id" class="chk">
            <input type="checkbox" :value="a.id" v-model="form.application_ids" /> {{ a.name }}
          </label>
          <span v-if="!apps.length" class="muted">Belum ada aplikasi.</span>
        </div>
        <button class="primary" style="margin-top:12px">Buat (draft)</button>
      </form>
    </div>

    <div class="panel">
      <div class="table-wrap">
        <table>
          <thead><tr><th>Nama</th><th>Periode</th><th>Status</th><th></th></tr></thead>
          <tbody>
            <tr v-for="c in campaigns" :key="c.id">
              <td>{{ c.name }}</td>
              <td class="muted">{{ c.period_start || '—' }} → {{ c.period_end || '—' }}</td>
              <td><span :class="['badge', c.status]">{{ c.status }}</span></td>
              <td><router-link :to="`/campaigns/${c.id}`">Buka →</router-link></td>
            </tr>
            <tr v-if="!campaigns.length"><td colspan="4" class="muted">Belum ada campaign.</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

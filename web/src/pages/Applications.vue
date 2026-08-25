<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api.js'
import { isAdmin } from '../store.js'

const apps = ref([])
const error = ref('')
const form = ref({ name: '', description: '', owner: '', department: '' })

const q = ref('')
const deptFilter = ref('')
const filtered = computed(() =>
  apps.value.filter((a) => {
    const s = q.value.trim().toLowerCase()
    const okText = !s || [a.name, a.owner, a.description].some((f) => (f || '').toLowerCase().includes(s))
    const okDept = !deptFilter.value || a.department === deptFilter.value
    return okText && okDept
  })
)

async function load() {
  try {
    apps.value = await api.get('/applications')
  } catch (e) {
    error.value = e.message
  }
}

async function create() {
  error.value = ''
  try {
    await api.post('/applications', form.value)
    form.value = { name: '', description: '', owner: '', department: '' }
    await load()
  } catch (e) {
    error.value = e.message
  }
}

onMounted(load)
</script>

<template>
  <div class="container">
    <h1>Applications</h1>
    <div v-if="error" class="err">{{ error }}</div>

    <div class="panel" v-if="isAdmin()">
      <h2 style="margin-top:0">Tambah aplikasi</h2>
      <form @submit.prevent="create">
        <div class="row">
          <div><label>Nama * <input v-model="form.name" required /></label></div>
          <div><label>Owner (email) <input v-model="form.owner" placeholder="app owner → reviewer" /></label></div>
          <div><label>Departemen
            <select v-model="form.department">
              <option value="">—</option>
              <option value="keuangan">Keuangan</option>
              <option value="marketing">Marketing</option>
              <option value="hr">HR</option>
            </select>
          </label></div>
        </div>
        <label>Deskripsi
          <input v-model="form.description" />
        </label>
        <button class="primary" style="margin-top:12px">Tambah</button>
      </form>
    </div>

    <div class="panel">
      <div class="row" style="margin-bottom:12px; align-items:flex-end">
        <div><label>Cari <input v-model="q" placeholder="nama / owner / deskripsi" /></label></div>
        <div style="flex:0 0 200px"><label>Departemen
          <select v-model="deptFilter">
            <option value="">semua</option>
            <option value="keuangan">Keuangan</option>
            <option value="marketing">Marketing</option>
            <option value="hr">HR</option>
          </select>
        </label></div>
        <div style="flex:0 0 auto" class="muted">{{ filtered.length }} / {{ apps.length }}</div>
      </div>
      <div class="table-wrap">
        <table>
          <thead><tr><th>Nama</th><th>Departemen</th><th>Owner</th><th>Deskripsi</th><th></th></tr></thead>
          <tbody>
            <tr v-for="a in filtered" :key="a.id">
              <td><router-link :to="`/applications/${a.id}`">{{ a.name }}</router-link></td>
              <td><span v-if="a.department" :class="['badge', a.department]">{{ a.department }}</span><span v-else class="muted">—</span></td>
              <td class="muted">{{ a.owner || '—' }}</td>
              <td class="muted">{{ a.description || '—' }}</td>
              <td><router-link :to="`/applications/${a.id}`">Kelola →</router-link></td>
            </tr>
            <tr v-if="!filtered.length"><td colspan="5" class="muted">{{ apps.length ? 'Tidak ada aplikasi cocok.' : 'Belum ada aplikasi.' }}</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

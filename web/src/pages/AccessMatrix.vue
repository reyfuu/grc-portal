<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api.js'

const route = useRoute()
const apps = ref([])
const selected = ref('')
const matrix = ref(null)
const error = ref('')
const q = ref('')

const filteredRows = computed(() => {
  if (!matrix.value) return []
  const s = q.value.trim().toLowerCase()
  return s ? matrix.value.rows.filter((r) => r.subject.toLowerCase().includes(s)) : matrix.value.rows
})

async function loadMatrix() {
  matrix.value = null
  if (!selected.value) return
  try {
    matrix.value = await api.get(`/matrix?application_id=${selected.value}`)
  } catch (e) {
    error.value = e.message
  }
}

function hasRole(row, roleId) {
  return row.role_ids.includes(roleId)
}

watch(selected, loadMatrix)

onMounted(async () => {
  try {
    apps.value = await api.get('/applications')
    if (route.query.app) selected.value = String(route.query.app)
    else if (apps.value.length) selected.value = String(apps.value[0].id)
  } catch (e) {
    error.value = e.message
  }
})
</script>

<template>
  <div class="container">
    <h1>Access Matrix</h1>
    <p class="muted">Siapa punya role apa di sebuah aplikasi (akses aktif).</p>
    <div v-if="error" class="err">{{ error }}</div>

    <div class="panel">
      <label>Aplikasi
        <select v-model="selected" style="max-width:320px">
          <option v-for="a in apps" :key="a.id" :value="String(a.id)">{{ a.name }}</option>
        </select>
      </label>
    </div>

    <div class="panel" v-if="matrix">
      <div class="row" style="margin-bottom:12px; align-items:flex-end">
        <div style="max-width:320px"><label>Cari subject <input v-model="q" placeholder="email / username" /></label></div>
        <div style="flex:0 0 auto" class="muted">{{ filteredRows.length }} / {{ matrix.rows.length }} subject</div>
      </div>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th style="text-align:left">Subject</th>
              <th v-for="r in matrix.roles" :key="r.id" style="text-align:center; width:96px; white-space:nowrap">{{ r.name }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in filteredRows" :key="row.subject_id">
              <td>{{ row.subject }}</td>
              <td v-for="r in matrix.roles" :key="r.id" style="text-align:center; vertical-align:middle; width:96px">
                <span v-if="hasRole(row, r.id)" class="check">✓</span>
                <span v-else class="muted">·</span>
              </td>
            </tr>
            <tr v-if="matrix.rows.length && !filteredRows.length">
              <td :colspan="matrix.roles.length + 1" class="muted">Tidak ada subject cocok.</td>
            </tr>
            <tr v-if="!matrix.rows.length">
              <td :colspan="matrix.roles.length + 1" class="muted">Belum ada akses aktif untuk aplikasi ini.</td>
            </tr>
            <tr v-if="!matrix.roles.length">
              <td colspan="2" class="muted">Aplikasi ini belum punya role.</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api.js'
import { isAdmin } from '../store.js'

const data = ref({ rows: [], counts: { active: 0, inactive: 0, not_in_hr: 0, service: 0 } })
const showAll = ref(false)
const error = ref('')

const hrList = ref([])
const form = ref({ email: '', username: '', display_name: '', department: '', status: 'active' })
const saving = ref(false)
const msg = ref('')

const file = ref(null)
const importResult = ref(null)
const importing = ref(false)

const statusMeta = {
  active: { label: 'Aktif', desc: 'karyawan aktif di HR' },
  inactive: { label: 'Nonaktif', desc: 'nonaktif di HR tapi akses masih aktif' },
  not_in_hr: { label: 'Tidak ada di HR', desc: 'punya akses tapi tak ditemukan di HR (mismatch)' },
  service: { label: 'Service account', desc: 'bukan karyawan, wajar tak ada di HR' },
}

const flaggedRows = computed(() => data.value.rows.filter((r) => r.flagged))
const displayRows = computed(() => (showAll.value ? data.value.rows : flaggedRows.value))
const tiles = computed(() => [
  { key: 'inactive', accent: 'danger' },
  { key: 'not_in_hr', accent: 'warn' },
  { key: 'active', accent: 'ok' },
  { key: 'service', accent: 'muted' },
].map((t) => ({ ...t, value: data.value.counts[t.key] || 0, ...statusMeta[t.key] })))

const importHeader = 'email,username,display_name,department,status'
const importSample =
  importHeader +
  '\nana@co.id,,Ana Putri,Keuangan,active' +
  '\nbudi@co.id,,Budi (resign),IT,inactive'
const importHref = 'data:text/csv;charset=utf-8,' + encodeURIComponent(importSample)

async function loadReview() {
  try {
    data.value = await api.get('/access-review')
  } catch (e) {
    error.value = e.message
  }
}
async function loadHR() {
  try {
    hrList.value = await api.get('/hr-employees')
  } catch (e) {
    error.value = e.message
  }
}

async function addHR() {
  error.value = ''
  msg.value = ''
  saving.value = true
  try {
    await api.post('/hr-employees', form.value)
    form.value = { email: '', username: '', display_name: '', department: '', status: 'active' }
    msg.value = 'Karyawan HR ditambahkan.'
    await Promise.all([loadHR(), loadReview()])
  } catch (e) {
    error.value = e.message
  } finally {
    saving.value = false
  }
}

async function removeHR(e) {
  if (!confirm(`Hapus data HR "${e.display_name || e.email || e.username}"?`)) return
  try {
    await api.del(`/hr-employees/${e.id}`)
    await Promise.all([loadHR(), loadReview()])
  } catch (err) {
    error.value = err.message
  }
}

async function uploadHR() {
  error.value = ''
  importResult.value = null
  if (!file.value) { error.value = 'Pilih file CSV dulu'; return }
  importing.value = true
  try {
    const fd = new FormData()
    fd.append('file', file.value)
    importResult.value = await api.upload('/import/hr', fd)
    await Promise.all([loadHR(), loadReview()])
  } catch (e) {
    error.value = e.message
  } finally {
    importing.value = false
  }
}

onMounted(() => { loadReview(); loadHR() })
</script>

<template>
  <div class="container">
    <h1>Access Review <span class="muted" style="font-size:14px">/ rekonsiliasi vs HR</span></h1>
    <p class="muted">
      Data akses dikonsolidasikan lalu dicocokkan ke database HR (aktif/nonaktif). Akun nonaktif yang
      masih punya akses dan akun yang tak ada di HR ditandai sebagai <strong>daftar konfirmasi</strong>.
      Keputusan pencabutan akses tetap lewat approval di menu <router-link to="/campaigns">Campaigns</router-link>.
    </p>
    <div v-if="error" class="err">{{ error }}</div>

    <!-- Summary -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-4">
      <div v-for="t in tiles" :key="t.key" class="panel !mb-0 flex flex-col gap-1">
        <span class="text-xs uppercase tracking-wide text-muted">{{ t.label }}</span>
        <span class="text-3xl font-bold tabular-nums" :class="`text-${t.accent === 'muted' ? 'muted' : t.accent}`">{{ t.value }}</span>
        <span class="text-xs text-muted">{{ t.desc }}</span>
      </div>
    </div>

    <!-- Confirmation list -->
    <div class="panel">
      <div class="row" style="align-items:center; justify-content:space-between">
        <h2 style="margin:0">{{ showAll ? 'Semua akun berakses' : 'Daftar konfirmasi' }} ({{ displayRows.length }})</h2>
        <label class="muted" style="margin:0; display:flex; align-items:center; gap:6px">
          <input type="checkbox" v-model="showAll" style="width:auto" /> tampilkan semua akun
        </label>
      </div>
      <p v-if="!displayRows.length" class="muted" style="margin-top:8px">
        {{ showAll ? 'Belum ada akun berakses. Import binding akses dulu.' : 'Tidak ada akun bermasalah 🎉 — semua akses cocok dengan HR aktif.' }}
      </p>
      <div class="table-wrap" v-else>
        <table>
          <thead><tr><th>Subject</th><th>Tipe</th><th>Status HR</th><th>Departemen</th><th class="tabular-nums">Akses aktif</th></tr></thead>
          <tbody>
            <tr v-for="r in displayRows" :key="r.subject_id">
              <td><strong>{{ r.subject }}</strong></td>
              <td class="muted">{{ r.type === 'service_account' ? 'service' : 'user' }}</td>
              <td><span :class="['badge', r.hr_status]">{{ statusMeta[r.hr_status].label }}</span></td>
              <td class="muted">{{ r.department || '—' }}</td>
              <td class="tabular-nums">{{ r.active_bindings }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- HR directory management -->
    <div class="panel" v-if="isAdmin()">
      <h2 style="margin-top:0">Database HR</h2>
      <div v-if="msg" class="ok">{{ msg }}</div>
      <form @submit.prevent="addHR">
        <div class="row">
          <div><label>Email <input v-model="form.email" placeholder="nama@co.id" /></label></div>
          <div><label>Username <input v-model="form.username" placeholder="kalau tanpa email" /></label></div>
          <div><label>Nama <input v-model="form.display_name" /></label></div>
          <div style="flex:0 0 160px"><label>Departemen <input v-model="form.department" /></label></div>
          <div style="flex:0 0 140px"><label>Status
            <select v-model="form.status"><option value="active">Aktif</option><option value="inactive">Nonaktif</option></select>
          </label></div>
        </div>
        <button class="primary" style="margin-top:12px" :disabled="saving">{{ saving ? 'Menyimpan…' : 'Tambah karyawan' }}</button>
      </form>

      <h3 style="margin:20px 0 6px; font-size:14px">Import CSV</h3>
      <p class="muted" style="margin:0 0 6px">Header: <code>{{ importHeader }}</code> · <a :href="importHref" download="hr.csv">contoh</a></p>
      <div class="row" style="align-items:center">
        <div><input type="file" accept=".csv,text/csv" @change="(e) => (file = e.target.files[0])" /></div>
        <div style="flex:0"><button :disabled="importing" @click="uploadHR">{{ importing ? 'Mengunggah…' : 'Upload' }}</button></div>
      </div>
      <div v-if="importResult" class="ok" style="margin-top:8px">Imported: {{ importResult.imported }} · Error: {{ importResult.errors.length }}</div>

      <div class="table-wrap" style="margin-top:16px">
        <table>
          <thead><tr><th>Nama</th><th>Email / username</th><th>Departemen</th><th>Status</th><th></th></tr></thead>
          <tbody>
            <tr v-for="e in hrList" :key="e.id">
              <td>{{ e.display_name || '—' }}</td>
              <td class="muted">{{ e.email || e.username }}</td>
              <td class="muted">{{ e.department || '—' }}</td>
              <td><span :class="['badge', e.status]">{{ e.status === 'active' ? 'aktif' : 'nonaktif' }}</span></td>
              <td><button class="btn-sm danger" @click="removeHR(e)">Hapus</button></td>
            </tr>
            <tr v-if="!hrList.length"><td colspan="5" class="muted">Belum ada data HR. Tambah atau import CSV.</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

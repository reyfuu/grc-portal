<script setup>
import { ref } from 'vue'
import { api } from '../api.js'

const file = ref(null)
const result = ref(null)
const error = ref('')
const busy = ref(false)

const header = 'application,subject_email,subject_username,subject_display_name,subject_type,role,granted_at'
const sample =
  header +
  '\nSalesforce,ana@co.id,,Ana Putri,user,admin,2026-01-10' +
  '\nSalesforce,,svc_backup,Backup Service,service_account,viewer,'
const sampleHref = 'data:text/csv;charset=utf-8,' + encodeURIComponent(sample)

function pick(e) { file.value = e.target.files[0] }

async function upload() {
  error.value = ''
  result.value = null
  if (!file.value) { error.value = 'Pilih file CSV dulu'; return }
  busy.value = true
  try {
    const form = new FormData()
    form.append('file', file.value)
    result.value = await api.upload('/import/bindings', form)
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="container">
    <h1>Import akses (CSV)</h1>
    <div v-if="error" class="err">{{ error }}</div>

    <div class="panel">
      <p class="muted">Satu baris = satu akses (binding). Aplikasi harus sudah dibuat lebih dulu.</p>
      <p>Header wajib:</p>
      <pre style="background:#f2f4f8;padding:10px;border-radius:6px;overflow-x:auto">{{ header }}</pre>
      <p><a :href="sampleHref" download="sample.csv">Unduh contoh CSV</a></p>
      <label>File CSV
        <input type="file" accept=".csv,text/csv" @change="pick" />
      </label>
      <button class="primary" style="margin-top:12px" :disabled="busy" @click="upload">
        {{ busy ? 'Mengunggah…' : 'Upload' }}
      </button>
    </div>

    <div class="panel" v-if="result">
      <div class="ok">Imported: {{ result.imported }} · Skipped (duplikat): {{ result.skipped }} · Error: {{ result.errors.length }}</div>
      <div v-if="result.errors.length" class="table-wrap">
        <table>
          <thead><tr><th>Baris</th><th>Masalah</th></tr></thead>
          <tbody>
            <tr v-for="(e, i) in result.errors" :key="i"><td>{{ e.line }}</td><td>{{ e.msg }}</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

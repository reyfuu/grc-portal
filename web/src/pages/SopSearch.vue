<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api.js'
import { isAdmin } from '../store.js'

const question = ref('')
const askCategory = ref('')
const asking = ref(false)
const result = ref(null) // { answer, sources, ai_enabled, model }
const error = ref('')

const docs = ref([])
const form = ref({ title: '', category: 'sop', source: '', body: '' })
const saving = ref(false)
const msg = ref('')

async function loadDocs() {
  try {
    docs.value = await api.get('/documents')
  } catch (e) {
    error.value = e.message
  }
}

async function ask() {
  if (!question.value.trim()) return
  error.value = ''
  asking.value = true
  result.value = null
  try {
    result.value = await api.post('/documents/ask', { question: question.value, category: askCategory.value })
  } catch (e) {
    error.value = e.message
  } finally {
    asking.value = false
  }
}

async function addDoc() {
  error.value = ''
  msg.value = ''
  saving.value = true
  try {
    await api.post('/documents', form.value)
    msg.value = 'Dokumen ditambahkan.'
    form.value = { title: '', category: 'sop', source: '', body: '' }
    await loadDocs()
  } catch (e) {
    error.value = e.message
  } finally {
    saving.value = false
  }
}

async function removeDoc(d) {
  if (!confirm(`Hapus dokumen "${d.title}"?`)) return
  try {
    await api.del(`/documents/${d.id}`)
    await loadDocs()
  } catch (e) {
    error.value = e.message
  }
}

onMounted(loadDocs)
</script>

<template>
  <div class="container">
    <h1>SOP &amp; Kebijakan <span class="muted" style="font-size:14px">/ pencarian AI</span></h1>
    <p class="muted">Tanya prosedur atau kebijakan internal — dijawab AI berdasarkan dokumen yang tersimpan, dengan referensi.</p>
    <div v-if="error" class="err">{{ error }}</div>

    <!-- Ask (RAG) -->
    <div class="panel">
      <form @submit.prevent="ask">
        <label>Pertanyaan
          <input v-model="question" placeholder="mis. Berapa lama proses approval cuti?" />
        </label>
        <div class="row" style="margin-top:8px; align-items:flex-end">
          <div style="flex:0 0 200px"><label>Kategori
            <select v-model="askCategory">
              <option value="">semua</option>
              <option value="sop">SOP</option>
              <option value="kebijakan">Kebijakan</option>
              <option value="evidence">Evidence</option>
            </select>
          </label></div>
          <div style="flex:0"><button class="primary" :disabled="asking">{{ asking ? 'Mencari…' : 'Tanya AI' }}</button></div>
        </div>
      </form>

      <div v-if="asking" class="panel !mb-0" style="margin-top:16px; background:#f7f9ff; border-color:#dbe4ff">
        <div class="flex items-center gap-2 text-muted"><span class="spinner"></span> Mencari jawaban di dokumen…</div>
      </div>

      <div v-else-if="result" style="margin-top:16px">
        <div v-if="result.answer" class="panel !mb-0" style="background:#f7f9ff; border-color:#dbe4ff">
          <div class="flex items-center justify-between mb-1">
            <div class="text-xs uppercase tracking-wide text-muted">Jawaban AI</div>
            <div v-if="result.model" class="text-xs text-muted">dijawab oleh {{ result.model }}</div>
          </div>
          <div style="white-space:pre-wrap">{{ result.answer }}</div>
        </div>
        <div v-else-if="!result.ai_enabled" class="muted">AI belum aktif (set <code>GEMINI_API_KEY</code>). Menampilkan dokumen relevan saja.</div>
        <div v-else-if="result.error" class="err">AI gagal menjawab: {{ result.error }} — dokumen relevan tetap di bawah.</div>

        <div style="margin-top:12px">
          <div class="text-xs uppercase tracking-wide text-muted mb-2">Referensi ({{ result.sources.length }})</div>
          <p v-if="!result.sources.length" class="muted">Tidak ada dokumen yang cocok.</p>
          <div v-for="s in result.sources" :key="s.id" class="panel !mb-0" style="margin-bottom:8px">
            <div><span :class="['badge', s.category]">{{ s.category }}</span> <strong>{{ s.title }}</strong></div>
            <div class="muted" style="font-size:13px; margin-top:4px">{{ s.snippet }}</div>
          </div>
        </div>
      </div>
    </div>

    <!-- Manage documents -->
    <div class="panel" v-if="isAdmin()">
      <h2 style="margin-top:0">Tambah dokumen</h2>
      <div v-if="msg" class="ok">{{ msg }}</div>
      <form @submit.prevent="addDoc">
        <div class="row">
          <div><label>Judul * <input v-model="form.title" required /></label></div>
          <div style="flex:0 0 180px"><label>Kategori
            <select v-model="form.category">
              <option value="sop">SOP</option>
              <option value="kebijakan">Kebijakan</option>
              <option value="evidence">Evidence</option>
            </select>
          </label></div>
          <div><label>Sumber <input v-model="form.source" placeholder="mis. HR-SOP-01" /></label></div>
        </div>
        <label>Isi dokumen *
          <textarea v-model="form.body" rows="5" required placeholder="Tempel isi SOP / kebijakan…"></textarea>
        </label>
        <button class="primary" style="margin-top:12px" :disabled="saving">{{ saving ? 'Menyimpan…' : 'Tambah' }}</button>
      </form>
    </div>

    <div class="panel">
      <h2 style="margin-top:0">Dokumen ({{ docs.length }})</h2>
      <div class="table-wrap">
        <table>
          <thead><tr><th>Judul</th><th>Kategori</th><th>Sumber</th><th>Ditambahkan</th><th v-if="isAdmin()"></th></tr></thead>
          <tbody>
            <tr v-for="d in docs" :key="d.id">
              <td>{{ d.title }}</td>
              <td><span :class="['badge', d.category]">{{ d.category }}</span></td>
              <td class="muted">{{ d.source || '—' }}</td>
              <td class="muted">{{ new Date(d.created_at).toLocaleDateString() }}</td>
              <td v-if="isAdmin()"><button class="btn-sm danger" @click="removeDoc(d)">Hapus</button></td>
            </tr>
            <tr v-if="!docs.length"><td :colspan="isAdmin() ? 5 : 4" class="muted">Belum ada dokumen. Tambah SOP/kebijakan dulu.</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

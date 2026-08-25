<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api.js'
import { store, isAdmin } from '../store.js'

const canReview = computed(() => ['admin', 'reviewer'].includes(store.user?.role))

const requesterLabels = {
  auditor: 'Auditor', pihak_ketiga: 'Pihak Ketiga',
  internal_audit: 'Internal Audit', assessor: 'Assessor',
}

const list = ref([])
const error = ref('')
const msg = ref('')

const form = ref({ title: '', requester: 'auditor', description: '' })
const saving = ref(false)

const sel = ref(null) // { request, matches, similar }
const owner = ref('')
const note = ref('')
const busy = ref(false)
const owners = ref([]) // known application owners, to route gaps to the right person

async function loadList() {
  try {
    list.value = await api.get('/evidence-requests')
  } catch (e) {
    error.value = e.message
  }
}

async function loadOwners() {
  try {
    const apps = await api.get('/applications')
    owners.value = [...new Set(apps.map((a) => a.owner).filter(Boolean))].sort()
  } catch (e) { /* owner suggestions are optional */ }
}

async function createRequest() {
  error.value = ''
  msg.value = ''
  saving.value = true
  try {
    const { id } = await api.post('/evidence-requests', form.value)
    form.value = { title: '', requester: 'auditor', description: '' }
    await loadList()
    await select(id)
    msg.value = 'Request dibuat. Cek saran evidence untuk reuse di bawah.'
  } catch (e) {
    error.value = e.message
  } finally {
    saving.value = false
  }
}

async function select(id) {
  error.value = ''
  owner.value = ''
  note.value = ''
  try {
    sel.value = await api.get(`/evidence-requests/${id}`)
  } catch (e) {
    error.value = e.message
  }
}

async function resolve(action, payload) {
  busy.value = true
  error.value = ''
  try {
    await api.post(`/evidence-requests/${sel.value.request.id}/resolve`, { action, ...payload })
    await Promise.all([loadList(), select(sel.value.request.id)])
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}

async function remove(r) {
  if (!confirm(`Hapus request "${r.title}"?`)) return
  try {
    await api.del(`/evidence-requests/${r.id}`)
    if (sel.value && sel.value.request.id === r.id) sel.value = null
    await loadList()
  } catch (e) {
    error.value = e.message
  }
}

onMounted(() => { loadList(); loadOwners() })
</script>

<template>
  <div class="container">
    <h1>Evidence Management <span class="muted" style="font-size:14px">/ audit &amp; assessment</span></h1>
    <p class="muted">
      Request evidence dari auditor/assessor dicocokkan ke evidence yang pernah dipakai (reuse, dengan
      jejak sumber). Sistem menyarankan; <strong>manusia yang memutuskan</strong> — pakai kembali atau tandai gap
      lalu arahkan ke owner.
    </p>
    <div v-if="error" class="err">{{ error }}</div>

    <div class="panel" v-if="isAdmin()">
      <h2 style="margin-top:0">Request baru</h2>
      <div v-if="msg" class="ok">{{ msg }}</div>
      <form @submit.prevent="createRequest">
        <div class="row">
          <div><label>Judul / evidence yang diminta *
            <input v-model="form.title" required placeholder="mis. Bukti review akses privileged Q2" />
          </label></div>
          <div style="flex:0 0 200px"><label>Peminta
            <select v-model="form.requester">
              <option v-for="(lbl, val) in requesterLabels" :key="val" :value="val">{{ lbl }}</option>
            </select>
          </label></div>
        </div>
        <label>Detail permintaan
          <textarea v-model="form.description" rows="3" placeholder="Konteks / periode / sistem terkait…"></textarea>
        </label>
        <button class="primary" style="margin-top:12px" :disabled="saving">{{ saving ? 'Menyimpan…' : 'Buat request' }}</button>
      </form>
    </div>

    <div class="panel">
      <h2 style="margin-top:0">Request ({{ list.length }})</h2>
      <div class="table-wrap">
        <table>
          <thead><tr><th>Judul</th><th>Peminta</th><th>Status</th><th>Sumber / arah</th><th>Dibuat</th><th></th></tr></thead>
          <tbody>
            <tr v-for="r in list" :key="r.id" @click="select(r.id)" style="cursor:pointer"
                :class="{ 'row-sel': sel && sel.request.id === r.id }">
              <td><strong>{{ r.title }}</strong></td>
              <td class="muted">{{ requesterLabels[r.requester] || r.requester }}</td>
              <td><span :class="['badge', r.status]">{{ r.status }}</span></td>
              <td class="muted">{{ r.document_title || r.owner || '—' }}</td>
              <td class="muted">{{ new Date(r.created_at).toLocaleDateString() }}</td>
              <td v-if="isAdmin()"><button class="btn-sm danger" @click.stop="remove(r)">Hapus</button></td>
            </tr>
            <tr v-if="!list.length"><td colspan="6" class="muted">Belum ada request. Buat request evidence dulu.</td></tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Detail + resolusi -->
    <div class="panel" v-if="sel">
      <div class="row" style="align-items:baseline; justify-content:space-between">
        <h2 style="margin:0">{{ sel.request.title }} <span :class="['badge', sel.request.status]">{{ sel.request.status }}</span></h2>
        <button class="btn-sm" @click="sel = null">Tutup</button>
      </div>
      <p v-if="sel.request.description" class="muted" style="margin-top:4px">{{ sel.request.description }}</p>

      <!-- Sudah diputuskan -->
      <div v-if="sel.request.status === 'fulfilled'" class="ok" style="margin-top:12px">
        Dipenuhi dengan reuse — jejak sumber: <strong>{{ sel.request.document_title }}</strong>.
        <span v-if="sel.request.note"> Catatan: {{ sel.request.note }}</span>
      </div>
      <div v-else-if="sel.request.status === 'gap'" class="err" style="margin-top:12px">
        Ditandai gap → diarahkan ke <strong>{{ sel.request.owner }}</strong>.
        <span v-if="sel.request.note"> Catatan: {{ sel.request.note }}</span>
      </div>

      <!-- Masih open -->
      <template v-else>
        <div v-if="sel.similar.length" style="margin-top:16px">
          <div class="text-xs uppercase tracking-wide text-muted mb-2">⚠ Request serupa (kemungkinan duplikat) — {{ sel.similar.length }}</div>
          <div v-for="s in sel.similar" :key="s.id" class="panel !mb-0" style="margin-bottom:8px; cursor:pointer" @click="select(s.id)">
            <span :class="['badge', s.status]">{{ s.status }}</span> <strong>{{ s.title }}</strong>
            <span class="muted" v-if="s.document_title"> — sudah dipenuhi: {{ s.document_title }}</span>
          </div>
        </div>

        <div style="margin-top:16px">
          <div class="text-xs uppercase tracking-wide text-muted mb-2">Evidence yang bisa dipakai kembali ({{ sel.matches.length }})</div>
          <p v-if="!sel.matches.length" class="muted">Tidak ada evidence cocok. Kemungkinan gap — arahkan ke owner di bawah.</p>
          <div v-for="m in sel.matches" :key="m.id" class="panel !mb-0" style="margin-bottom:8px">
            <div class="row" style="align-items:center; justify-content:space-between">
              <div style="flex:1">
                <div><span :class="['badge', m.category]">{{ m.category }}</span> <strong>{{ m.title }}</strong></div>
                <div class="muted" style="font-size:13px; margin-top:4px">{{ m.snippet }}</div>
              </div>
              <button v-if="canReview" class="primary btn-sm" :disabled="busy"
                      @click="resolve('fulfilled', { document_id: m.id, note })">Pakai sebagai sumber</button>
            </div>
          </div>
        </div>

        <div v-if="canReview" class="panel !mb-0" style="margin-top:16px; background:#fafbfc">
          <div class="text-xs uppercase tracking-wide text-muted mb-2">Tidak ada yang cocok? Tandai gap &amp; arahkan</div>
          <div class="row" style="align-items:flex-end">
            <div><label>Owner tujuan *
              <input v-model="owner" list="owner-options" placeholder="mis. Infrastructure / Developer" />
              <datalist id="owner-options"><option v-for="o in owners" :key="o" :value="o" /></datalist>
            </label></div>
            <div><label>Catatan
              <input v-model="note" placeholder="opsional" />
            </label></div>
            <div style="flex:0"><button class="danger" :disabled="busy || !owner.trim()"
                    @click="resolve('gap', { owner, note })">Tandai gap</button></div>
          </div>
        </div>
        <p v-else class="muted" style="margin-top:12px">Hanya reviewer/admin yang dapat memutuskan reuse atau gap.</p>
      </template>
    </div>
  </div>
</template>

<style scoped>
.row-sel { background: #eef2ff; }
</style>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api.js'
import { store, isAdmin } from '../store.js'

const route = useRoute()
const id = route.params.id
const data = ref(null)
const items = ref([])
const error = ref('')
const filterMine = ref(false)
const filterDecision = ref('')

const revoking = ref(null) // item being revoked
const justification = ref('')

const canDecide = computed(() => ['admin', 'reviewer'].includes(store.user?.role))
const isActive = computed(() => data.value?.campaign.status === 'active')

async function load() {
  error.value = ''
  try {
    data.value = await api.get(`/campaigns/${id}`)
    await loadItems()
  } catch (e) {
    error.value = e.message
  }
}

async function loadItems() {
  const qs = []
  if (filterMine.value) qs.push('reviewer=me')
  if (filterDecision.value) qs.push('decision=' + filterDecision.value)
  items.value = await api.get(`/campaigns/${id}/items${qs.length ? '?' + qs.join('&') : ''}`)
}

async function activate() {
  if (!confirm('Aktifkan campaign? Akses aktif akan di-snapshot jadi item review.')) return
  try { await api.post(`/campaigns/${id}/activate`); await load() } catch (e) { error.value = e.message }
}
async function close() {
  if (!confirm('Tutup campaign? Setelah ditutup tidak bisa diputus lagi.')) return
  try { await api.post(`/campaigns/${id}/close`); await load() } catch (e) { error.value = e.message }
}
async function bulkCertify() {
  if (!confirm('Certify semua item pending yang jadi tanggung jawab kamu?')) return
  try { await api.post(`/campaigns/${id}/items/bulk-decide`, { decision: 'certified' }); await load() } catch (e) { error.value = e.message }
}
async function certify(item) {
  try { await api.post(`/review-items/${item.id}/decide`, { decision: 'certified' }); await load() } catch (e) { error.value = e.message }
}
function openRevoke(item) { revoking.value = item; justification.value = '' }
async function confirmRevoke() {
  if (!justification.value.trim()) { error.value = 'Justifikasi wajib diisi untuk revoke'; return }
  try {
    await api.post(`/review-items/${revoking.value.id}/decide`, { decision: 'revoked', justification: justification.value })
    revoking.value = null
    await load()
  } catch (e) { error.value = e.message }
}

onMounted(load)
</script>

<template>
  <div class="container" v-if="data">
    <h1>
      {{ data.campaign.name }}
      <span :class="['badge', data.campaign.status]" style="font-size:12px;vertical-align:middle">{{ data.campaign.status }}</span>
    </h1>
    <p><router-link to="/campaigns">← Campaigns</router-link></p>
    <div v-if="error" class="err">{{ error }}</div>

    <div class="panel">
      <div class="row">
        <div><strong>Total</strong><br />{{ data.counts.total }}</div>
        <div><strong>Pending</strong><br /><span class="badge pending">{{ data.counts.pending }}</span></div>
        <div><strong>Certified</strong><br /><span class="badge certified">{{ data.counts.certified }}</span></div>
        <div><strong>Revoked</strong><br /><span class="badge revoked">{{ data.counts.revoked }}</span></div>
        <div>
          <strong>Aplikasi</strong><br />
          <span class="muted">{{ data.applications.map((a) => a.name).join(', ') || '—' }}</span>
        </div>
      </div>
      <div style="margin-top:14px; display:flex; gap:8px; flex-wrap:wrap">
        <button v-if="isAdmin() && data.campaign.status === 'draft'" class="primary" @click="activate">Aktifkan (snapshot)</button>
        <button v-if="isAdmin() && isActive" @click="close">Tutup campaign</button>
        <button v-if="canDecide && isActive" @click="bulkCertify">Certify semua pending (punya saya)</button>
        <a :href="`/api/campaigns/${id}/export`"><button>Export hasil (CSV)</button></a>
        <a :href="`/api/campaigns/${id}/export?decision=revoked`"><button>Export revoke-list</button></a>
      </div>
    </div>

    <div class="panel" v-if="data.campaign.status !== 'draft'">
      <div style="display:flex; gap:16px; align-items:center; flex-wrap:wrap">
        <h2 style="margin:0">Item review</h2>
        <label style="display:flex; gap:6px; align-items:center; margin:0">
          <input type="checkbox" v-model="filterMine" @change="loadItems" style="width:auto" /> hanya punya saya
        </label>
        <select v-model="filterDecision" @change="loadItems" style="width:auto" aria-label="Filter keputusan">
          <option value="">semua</option>
          <option value="pending">pending</option>
          <option value="certified">certified</option>
          <option value="revoked">revoked</option>
        </select>
      </div>
      <div class="table-wrap" style="margin-top:12px">
        <table>
          <thead><tr><th>Subject</th><th>Aplikasi</th><th>Role</th><th>Reviewer</th><th>Keputusan</th><th></th></tr></thead>
          <tbody>
            <tr v-for="it in items" :key="it.id">
              <td>{{ it.subject }}</td><td>{{ it.app }}</td><td>{{ it.role }}</td>
              <td class="muted">{{ it.reviewer || '—' }}</td>
              <td>
                <span :class="['badge', it.decision]">{{ it.decision }}</span>
                <div v-if="it.justification" class="muted" style="font-size:12px">{{ it.justification }}</div>
              </td>
              <td>
                <template v-if="canDecide && isActive && it.decision === 'pending'">
                  <button class="btn-sm primary" @click="certify(it)">Certify</button>
                  <button class="btn-sm danger" @click="openRevoke(it)">Revoke</button>
                </template>
              </td>
            </tr>
            <tr v-if="!items.length"><td colspan="6" class="muted">Tidak ada item.</td></tr>
          </tbody>
        </table>
      </div>
    </div>

    <div class="modal-bg" v-if="revoking" @click.self="revoking = null">
      <div class="modal">
        <h2 style="margin-top:0">Revoke akses</h2>
        <p class="muted">{{ revoking.subject }} — {{ revoking.app }} / {{ revoking.role }}</p>
        <label>Justifikasi (wajib)
          <textarea v-model="justification" rows="3" placeholder="alasan akses dicabut"></textarea>
        </label>
        <div style="margin-top:14px; display:flex; gap:8px; justify-content:flex-end">
          <button @click="revoking = null">Batal</button>
          <button class="primary danger" @click="confirmRevoke">Revoke</button>
        </div>
      </div>
    </div>
  </div>
  <div class="container" v-else>
    <p><router-link to="/campaigns">← Campaigns</router-link></p>
    <div v-if="error" class="err">{{ error }}</div>
    <p v-else class="muted">Memuat…</p>
  </div>
</template>

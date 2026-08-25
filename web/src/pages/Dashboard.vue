<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api.js'

const campaigns = ref([])
const apps = ref([])
const subjects = ref([])
const bindings = ref([])
const evidence = ref([])
const review = ref({ counts: {} })
const error = ref('')
const loading = ref(true)

onMounted(async () => {
  // Parallel; tolerate a partial failure so the rest of the dashboard still renders.
  const [c, a, s, b, e, rv] = await Promise.allSettled([
    api.get('/campaigns'), api.get('/applications'), api.get('/subjects'),
    api.get('/bindings'), api.get('/evidence-requests'), api.get('/access-review'),
  ])
  if (c.status === 'fulfilled') campaigns.value = c.value
  if (a.status === 'fulfilled') apps.value = a.value
  if (s.status === 'fulfilled') subjects.value = s.value
  if (b.status === 'fulfilled') bindings.value = b.value
  if (e.status === 'fulfilled') evidence.value = e.value
  if (rv.status === 'fulfilled') review.value = rv.value
  const failed = [c, a, s, b, e, rv].find((r) => r.status === 'rejected')
  if (failed) error.value = failed.reason?.message || 'Gagal memuat sebagian data.'
  loading.value = false
})

const active = computed(() => campaigns.value.filter((c) => c.status === 'active'))
const openEvidence = computed(() => evidence.value.filter((x) => x.status === 'open').length)
const needConfirm = computed(() => (review.value.counts.inactive || 0) + (review.value.counts.not_in_hr || 0))
const stats = computed(() => [
  { label: 'Applications', value: apps.value.length, accent: 'brand', to: '/applications' },
  { label: 'Access bindings', value: bindings.value.length, accent: 'ink', to: '/matrix' },
  { label: 'Perlu konfirmasi', value: needConfirm.value, accent: 'warn', to: '/access-review' },
  { label: 'Campaign aktif', value: active.value.length, accent: 'ok', to: '/campaigns' },
  { label: 'Evidence open', value: openEvidence.value, accent: 'warn', to: '/evidence' },
])
const accentText = { brand: 'text-brand', ink: 'text-ink', warn: 'text-warn', ok: 'text-ok' }
const accentDot = { brand: 'bg-brand', ink: 'bg-ink', warn: 'bg-warn', ok: 'bg-ok' }
</script>

<template>
  <div class="container">
    <h1>Dashboard</h1>
    <div v-if="error" class="err">{{ error }}</div>

    <!-- Stat cards -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-4">
      <router-link
        v-for="s in stats" :key="s.label" :to="s.to"
        class="panel !mb-0 no-underline flex flex-col gap-1 transition hover:-translate-y-0.5 hover:shadow-md">
        <span class="flex items-center gap-2 text-xs uppercase tracking-wide text-muted">
          <span class="inline-block w-2 h-2 rounded-full" :class="accentDot[s.accent]"></span>{{ s.label }}
        </span>
        <span class="text-3xl font-bold tabular-nums" :class="accentText[s.accent]">
          {{ loading ? '·' : s.value }}
        </span>
      </router-link>
    </div>

    <div class="panel">
      <h2 style="margin-top:0">Campaign aktif</h2>
      <p v-if="!active.length" class="muted">Belum ada campaign aktif.</p>
      <div class="table-wrap" v-else>
        <table>
          <thead><tr><th>Nama</th><th>Periode</th><th>Status</th><th></th></tr></thead>
          <tbody>
            <tr v-for="c in active" :key="c.id">
              <td>{{ c.name }}</td>
              <td class="muted">{{ c.period_start || '—' }} → {{ c.period_end || '—' }}</td>
              <td><span :class="['badge', c.status]">{{ c.status }}</span></td>
              <td><router-link :to="`/campaigns/${c.id}`">Review →</router-link></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div class="panel">
      <h2 style="margin-top:0">Semua campaign</h2>
      <p v-if="!campaigns.length" class="muted">Belum ada campaign. Buat di menu Campaigns.</p>
      <div class="table-wrap" v-else>
        <table>
          <thead><tr><th>Nama</th><th>Status</th><th></th></tr></thead>
          <tbody>
            <tr v-for="c in campaigns" :key="c.id">
              <td>{{ c.name }}</td>
              <td><span :class="['badge', c.status]">{{ c.status }}</span></td>
              <td><router-link :to="`/campaigns/${c.id}`">Buka →</router-link></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

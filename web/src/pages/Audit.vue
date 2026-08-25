<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api.js'

const entries = ref([])
const error = ref('')

onMounted(async () => {
  try {
    entries.value = await api.get('/audit')
  } catch (e) {
    error.value = e.message
  }
})

function when(ts) {
  return new Date(ts).toLocaleString()
}

const keyOf = (d) => `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`

// Bar chart data: event count per day for the last 14 days (local time).
const byDay = computed(() => {
  const counts = {}
  for (const e of entries.value) counts[keyOf(new Date(e.created_at))] = (counts[keyOf(new Date(e.created_at))] || 0) + 1
  const days = []
  const now = new Date()
  for (let i = 13; i >= 0; i--) {
    const d = new Date(now)
    d.setDate(now.getDate() - i)
    days.push({ label: `${d.getDate()}/${d.getMonth() + 1}`, count: counts[keyOf(d)] || 0 })
  }
  return days
})
const dayMax = computed(() => Math.max(1, ...byDay.value.map((d) => d.count)))

// Horizontal bars: top actions by frequency.
const byAction = computed(() => {
  const counts = {}
  for (const e of entries.value) counts[e.action] = (counts[e.action] || 0) + 1
  return Object.entries(counts)
    .map(([action, count]) => ({ action, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 8)
})
const actionMax = computed(() => Math.max(1, ...byAction.value.map((a) => a.count)))

const actors = computed(() => new Set(entries.value.map((e) => e.actor).filter(Boolean)).size)

// Group audit activity by the department of the app it touches.
const DEPTS = [
  { key: 'keuangan', label: 'Keuangan', dot: 'bg-ok', bar: 'bg-ok' },
  { key: 'marketing', label: 'Marketing', dot: 'bg-brand', bar: 'bg-brand' },
  { key: 'hr', label: 'HR', dot: 'bg-warn', bar: 'bg-warn' },
  { key: 'lainnya', label: 'Lainnya', dot: 'bg-muted', bar: 'bg-muted' },
]
const deptRows = computed(() => {
  const c = {}
  for (const e of entries.value) {
    const d = e.department || 'lainnya'
    c[d] = (c[d] || 0) + 1
  }
  return DEPTS.map((d) => ({ ...d, count: c[d.key] || 0 }))
})
const deptMax = computed(() => Math.max(1, ...deptRows.value.map((d) => d.count)))

// Text alternatives so the CSS bar charts aren't invisible to screen readers.
const dayAria = computed(() => 'Aktivitas 14 hari terakhir. ' + byDay.value.map((d) => `${d.label}: ${d.count}`).join('; '))
const actionAria = computed(() => 'Aksi terbanyak. ' + byAction.value.map((a) => `${a.action}: ${a.count}`).join('; '))
const deptAria = computed(() => 'Aktivitas per departemen. ' + deptRows.value.map((d) => `${d.label}: ${d.count}`).join('; '))
</script>

<template>
  <div class="container">
    <h1>Audit log</h1>
    <p class="muted">500 kejadian terakhir.</p>
    <div v-if="error" class="err">{{ error }}</div>

    <!-- Summary + charts -->
    <div class="grid lg:grid-cols-3 gap-3 mb-4">
      <div class="panel !mb-0 flex flex-col justify-center gap-3">
        <div>
          <div class="text-xs uppercase tracking-wide text-muted">Total kejadian</div>
          <div class="text-3xl font-bold tabular-nums text-ink">{{ entries.length }}</div>
        </div>
        <div class="flex gap-6">
          <div>
            <div class="text-xs uppercase tracking-wide text-muted">Aktor</div>
            <div class="text-xl font-semibold tabular-nums text-brand">{{ actors }}</div>
          </div>
          <div>
            <div class="text-xs uppercase tracking-wide text-muted">Jenis aksi</div>
            <div class="text-xl font-semibold tabular-nums text-warn">{{ byAction.length }}</div>
          </div>
        </div>
      </div>

      <!-- Activity bars (14 days) -->
      <div class="panel !mb-0 lg:col-span-2">
        <div class="text-xs uppercase tracking-wide text-muted mb-3">Aktivitas 14 hari terakhir</div>
        <div class="flex items-end gap-1.5 h-28" role="img" :aria-label="dayAria">
          <div v-for="d in byDay" :key="d.label" class="flex-1 flex flex-col items-center justify-end h-full"
               :title="`${d.label}: ${d.count}`">
            <div class="w-full rounded-t bg-brand hover:brightness-110 transition-[height] min-h-[2px]"
                 :style="{ height: (d.count / dayMax * 100) + '%' }"></div>
          </div>
        </div>
        <div class="flex gap-1.5 mt-1">
          <div v-for="d in byDay" :key="d.label" class="flex-1 text-center text-[10px] text-muted tabular-nums">{{ d.label }}</div>
        </div>
      </div>
    </div>

    <!-- Per department -->
    <div class="panel">
      <div class="text-xs uppercase tracking-wide text-muted mb-3">Aktivitas per departemen</div>
      <div class="grid sm:grid-cols-2 lg:grid-cols-4 gap-3 mb-4">
        <div v-for="d in deptRows" :key="d.key" class="border border-line rounded-lg p-3 flex flex-col gap-1">
          <span class="flex items-center gap-2 text-xs uppercase tracking-wide text-muted">
            <span class="inline-block w-2 h-2 rounded-full" :class="d.dot"></span>{{ d.label }}
          </span>
          <span class="text-2xl font-bold tabular-nums text-ink">{{ d.count }}</span>
        </div>
      </div>
      <div class="flex flex-col gap-2" role="img" :aria-label="deptAria">
        <div v-for="d in deptRows" :key="d.key" class="flex items-center gap-3">
          <div class="w-24 shrink-0 text-sm">{{ d.label }}</div>
          <div class="flex-1 bg-line/60 rounded h-5 overflow-hidden">
            <div class="h-full rounded flex items-center justify-end pr-2 min-w-6" :class="d.bar"
                 :style="{ width: (d.count / deptMax * 100) + '%' }">
              <span class="text-[11px] font-semibold text-white tabular-nums">{{ d.count }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Top actions -->
    <div class="panel" v-if="byAction.length">
      <div class="text-xs uppercase tracking-wide text-muted mb-3">Aksi terbanyak</div>
      <div class="flex flex-col gap-2" role="img" :aria-label="actionAria">
        <div v-for="a in byAction" :key="a.action" class="flex items-center gap-3">
          <div class="w-40 shrink-0 truncate text-sm" :title="a.action">{{ a.action }}</div>
          <div class="flex-1 bg-line/60 rounded h-5 overflow-hidden">
            <div class="h-full bg-brand rounded flex items-center justify-end pr-2 min-w-[1.5rem]"
                 :style="{ width: (a.count / actionMax * 100) + '%' }">
              <span class="text-[11px] font-semibold text-white tabular-nums">{{ a.count }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="panel table-wrap">
      <table>
        <thead><tr><th>Waktu</th><th>Departemen</th><th>Aktor</th><th>Aksi</th><th>Entitas</th><th>Detail</th></tr></thead>
        <tbody>
          <tr v-for="e in entries" :key="e.id">
            <td class="muted">{{ when(e.created_at) }}</td>
            <td><span v-if="e.department" :class="['badge', e.department]">{{ e.department }}</span><span v-else class="muted">—</span></td>
            <td>{{ e.actor || '—' }}</td>
            <td>{{ e.action }}</td>
            <td class="muted">{{ e.entity_type }}<span v-if="e.entity_id">#{{ e.entity_id }}</span></td>
            <td class="muted">{{ e.detail }}</td>
          </tr>
          <tr v-if="!entries.length"><td colspan="6" class="muted">Belum ada aktivitas.</td></tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

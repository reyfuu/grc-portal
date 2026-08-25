<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api.js'
import { isAdmin } from '../store.js'

const route = useRoute()
const id = route.params.id
const app = ref(null)
const roles = ref([])
const bindings = ref([])
const error = ref('')

const roleForm = ref({ name: '', description: '' })
const grant = ref({ email: '', username: '', display_name: '', type: 'user', role_id: '' })

async function load() {
  error.value = ''
  try {
    ;[app.value, roles.value, bindings.value] = await Promise.all([
      api.get(`/applications/${id}`),
      api.get(`/applications/${id}/roles`),
      api.get(`/bindings?application_id=${id}`),
    ])
  } catch (e) {
    error.value = e.message
  }
}

async function addRole() {
  error.value = ''
  try {
    await api.post(`/applications/${id}/roles`, roleForm.value)
    roleForm.value = { name: '', description: '' }
    await load()
  } catch (e) {
    error.value = e.message
  }
}

async function addGrant() {
  error.value = ''
  if (!grant.value.role_id) { error.value = 'Pilih role dulu'; return }
  try {
    const subj = await api.post('/subjects', {
      email: grant.value.email, username: grant.value.username,
      display_name: grant.value.display_name, type: grant.value.type,
    })
    await api.post('/bindings', {
      application_id: Number(id), subject_id: subj.id, role_id: Number(grant.value.role_id),
    })
    grant.value = { email: '', username: '', display_name: '', type: 'user', role_id: '' }
    await load()
  } catch (e) {
    error.value = e.message
  }
}

async function removeBinding(b) {
  if (!confirm(`Hapus akses ${b.subject} — ${b.role}?`)) return
  try {
    await api.del(`/bindings/${b.id}`)
    await load()
  } catch (e) {
    error.value = e.message
  }
}

onMounted(load)
</script>

<template>
  <div class="container" v-if="app">
    <h1>{{ app.name }}
      <span v-if="app.department" :class="['badge', app.department]" style="font-size:12px; vertical-align:middle">{{ app.department }}</span>
      <span class="muted" style="font-size:14px">/ {{ app.owner || 'no owner' }}</span>
    </h1>
    <p><router-link to="/applications">← Applications</router-link>
      &nbsp;·&nbsp; <router-link :to="`/matrix?app=${id}`">Lihat di Access Matrix →</router-link></p>
    <div v-if="error" class="err">{{ error }}</div>

    <div class="panel">
      <h2 style="margin-top:0">Roles</h2>
      <div class="table-wrap">
        <table>
          <thead><tr><th>Role</th><th>Deskripsi</th></tr></thead>
          <tbody>
            <tr v-for="r in roles" :key="r.id"><td>{{ r.name }}</td><td class="muted">{{ r.description || '—' }}</td></tr>
            <tr v-if="!roles.length"><td colspan="2" class="muted">Belum ada role.</td></tr>
          </tbody>
        </table>
      </div>
      <form v-if="isAdmin()" @submit.prevent="addRole" class="row" style="margin-top:12px; align-items:flex-end">
        <div><label>Role baru <input v-model="roleForm.name" placeholder="admin / viewer / editor" required /></label></div>
        <div><label>Deskripsi <input v-model="roleForm.description" /></label></div>
        <div style="flex:0"><button class="primary">Tambah role</button></div>
      </form>
    </div>

    <div class="panel" v-if="isAdmin()">
      <h2 style="margin-top:0">Beri akses (grant)</h2>
      <form @submit.prevent="addGrant">
        <div class="row">
          <div><label>Email <input v-model="grant.email" type="email" /></label></div>
          <div><label>Username <input v-model="grant.username" placeholder="kalau tanpa email" /></label></div>
        </div>
        <div class="row">
          <div><label>Nama <input v-model="grant.display_name" /></label></div>
          <div><label>Tipe
            <select v-model="grant.type"><option value="user">user</option><option value="service_account">service_account</option></select>
          </label></div>
          <div><label>Role
            <select v-model="grant.role_id"><option value="">— pilih —</option>
              <option v-for="r in roles" :key="r.id" :value="r.id">{{ r.name }}</option></select>
          </label></div>
        </div>
        <button class="primary" style="margin-top:12px">Beri akses</button>
      </form>
    </div>

    <div class="panel">
      <h2 style="margin-top:0">Akses saat ini</h2>
      <div class="table-wrap">
        <table>
          <thead><tr><th>Subject</th><th>Role</th><th>Sumber</th><th>Status</th><th v-if="isAdmin()"></th></tr></thead>
          <tbody>
            <tr v-for="b in bindings" :key="b.id">
              <td>{{ b.subject }}</td><td>{{ b.role }}</td>
              <td class="muted">{{ b.source }}</td>
              <td><span :class="['badge', b.status === 'revoked' ? 'revoked' : 'active']">{{ b.status }}</span></td>
              <td v-if="isAdmin()"><button class="btn-sm danger" @click="removeBinding(b)">Hapus</button></td>
            </tr>
            <tr v-if="!bindings.length"><td colspan="5" class="muted">Belum ada akses.</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
  <div class="container" v-else>
    <p><router-link to="/applications">← Applications</router-link></p>
    <div v-if="error" class="err">{{ error }}</div>
    <p v-else class="muted">Memuat…</p>
  </div>
</template>

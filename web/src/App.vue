<script setup>
import { useRouter, useRoute } from 'vue-router'
import { store, isAdmin } from './store.js'
import { api } from './api.js'

const router = useRouter()
const route = useRoute()

async function logout() {
  await api.post('/logout')
  store.user = null
  router.push('/login')
}
</script>

<template>
  <nav class="nav" v-if="route.path !== '/login' && store.user">
    <span class="brand">GRC Portal</span>
    <router-link to="/">Dashboard</router-link>
    <router-link to="/applications">Applications</router-link>
    <router-link to="/matrix">Access Matrix</router-link>
    <router-link to="/access-review">Access Review</router-link>
    <router-link to="/campaigns">Campaigns</router-link>
    <router-link to="/sop">SOP &amp; Kebijakan</router-link>
    <router-link to="/evidence">Evidence</router-link>
    <router-link v-if="isAdmin()" to="/import">Import</router-link>
    <router-link v-if="store.user.role !== 'reviewer'" to="/audit">Audit</router-link>
    <span class="spacer"></span>
    <span class="who">{{ store.user.email }} ({{ store.user.role }})</span>
    <button class="btn-sm" @click="logout">Logout</button>
  </nav>
  <router-view />
</template>

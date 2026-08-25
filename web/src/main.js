import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import { routes } from './router.js'
import { store } from './store.js'
import { api } from './api.js'
import './style.css'

const router = createRouter({ history: createWebHistory(), routes })

router.beforeEach(async (to) => {
  if (to.path === '/login') return true
  if (!store.user) {
    try {
      store.user = await api.get('/me')
    } catch {
      return { path: '/login' }
    }
  }
  if (to.meta.roles && !to.meta.roles.includes(store.user.role)) return { path: '/' }
  return true
})

window.addEventListener('unauthorized', () => {
  store.user = null
  if (router.currentRoute.value.path !== '/login') router.push('/login')
})

createApp(App).use(router).mount('#app')

import { reactive } from 'vue'

// Single source of client state. The server is authoritative; this just caches the
// logged-in user for nav/guards.
export const store = reactive({ user: null })

export function isAdmin() {
  return store.user && store.user.role === 'admin'
}

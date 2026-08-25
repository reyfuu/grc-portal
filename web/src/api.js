// Thin fetch wrapper. Session cookie travels automatically (same-origin).
async function req(method, path, body, isForm) {
  const opts = { method, credentials: 'include', headers: {} }
  if (body !== undefined) {
    if (isForm) {
      opts.body = body
    } else {
      opts.headers['Content-Type'] = 'application/json'
      opts.body = JSON.stringify(body)
    }
  }
  const res = await fetch('/api' + path, opts)
  if (res.status === 401 && path !== '/me') {
    window.dispatchEvent(new Event('unauthorized'))
  }
  const ct = res.headers.get('content-type') || ''
  const data = ct.includes('application/json') ? await res.json() : await res.text()
  if (!res.ok) {
    throw new Error((data && data.error) || res.statusText)
  }
  return data
}

export const api = {
  get: (p) => req('GET', p),
  post: (p, b) => req('POST', p, b),
  patch: (p, b) => req('PATCH', p, b),
  del: (p) => req('DELETE', p),
  upload: (p, form) => req('POST', p, form, true),
}

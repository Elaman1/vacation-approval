import keycloak from './auth'

export async function api(path, options = {}) {
  try {
    await keycloak.updateToken(30)
  } catch {
    await keycloak.login()
    throw new Error('Сессия истекла. Необходимо войти снова.')
  }

  const response = await fetch(`/api${path}`, {
    ...options,
    headers: {
      'Authorization': `Bearer ${keycloak.token}`,
      'Content-Type': 'application/json',
      ...options.headers,
    },
  })
  const raw = await response.text()
  let body
  try { body = raw ? JSON.parse(raw) : null } catch { body = null }
  if (!response.ok) {
    throw new Error(body?.error || `Ошибка API: HTTP ${response.status}`)
  }
  return body
}

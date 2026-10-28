import React from 'react'
import { createRoot } from 'react-dom/client'
import keycloak from './auth'
import App from './App'
import './styles.css'

const root = createRoot(document.getElementById('root'))
root.render(<div className="page"><p>Выполняется вход...</p></div>)

keycloak.init({ onLoad: 'login-required', pkceMethod: 'S256', checkLoginIframe: false })
  .then((authenticated) => {
    if (!authenticated) {
      root.render(<div className="page">Требуется авторизация.</div>)
      return
    }
    root.render(<App />)
  })
  .catch((err) => {
    root.render(<div className="page"><h1>Ошибка входа</h1><p>{String(err)}</p></div>)
  })

import Keycloak from 'keycloak-js'

const keycloak = new Keycloak({
  url: import.meta.env.VITE_KEYCLOAK_URL || `${window.location.protocol}//${window.location.hostname}:8082`,
  realm: import.meta.env.VITE_KEYCLOAK_REALM || 'vacation',
  clientId: import.meta.env.VITE_KEYCLOAK_CLIENT_ID || 'vacation-web',
})

export default keycloak

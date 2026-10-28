import Keycloak from 'keycloak-js'

const keycloak = new Keycloak({
  url: 'http://localhost:8082',
  realm: 'vacation',
  clientId: 'vacation-web',
})

export default keycloak

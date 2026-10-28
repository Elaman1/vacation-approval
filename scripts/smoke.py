"""Read-only smoke check of a freshly bootstrapped Vacation Approval stack."""
import base64
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

API = os.getenv('SMOKE_API', 'http://api:8080')
KEYCLOAK = os.getenv('SMOKE_KEYCLOAK', 'http://keycloak:8080')
CAMUNDA = os.getenv('SMOKE_CAMUNDA', 'http://camunda:8080/engine-rest')
CLICKHOUSE = os.getenv('SMOKE_CLICKHOUSE', 'http://clickhouse:8123')
FRONTEND = os.getenv('SMOKE_FRONTEND', 'http://frontend')
CH_USER = os.getenv('CLICKHOUSE_USER', 'vacation')
CH_PASSWORD = os.getenv('CLICKHOUSE_PASSWORD', 'vacation')


def request(url, method='GET', data=None, username=None, password=None):
    req = urllib.request.Request(url, method=method, data=data)
    if username is not None:
        token = base64.b64encode(f'{username}:{password}'.encode()).decode()
        req.add_header('Authorization', 'Basic ' + token)
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as err:
        return err.code, err.read().decode()


def check(label, action, validator=lambda status, body: status == 200):
    deadline = time.monotonic() + 90
    last = 'not attempted'
    while time.monotonic() < deadline:
        try:
            status, body = action()
            if validator(status, body):
                print('OK ', label, flush=True)
                return
            last = f'HTTP {status}: {body[:180]}'
        except (OSError, ValueError) as exc:
            last = str(exc)
        time.sleep(2)
    raise RuntimeError(f'{label}: {last}')


def main():
    check('React frontend', lambda: request(FRONTEND + '/'),
          lambda s, b: s == 200 and 'id="root"' in b)
    check('Go API readiness', lambda: request(API + '/ready'),
          lambda s, b: s == 200 and 'ready' in b)
    check('Vacation endpoint rejects anonymous requests',
          lambda: request(API + '/vacations'), lambda s, _: s == 401)
    check('Manager endpoint rejects anonymous requests',
          lambda: request(API + '/manager/pending-vacations'), lambda s, _: s == 401)

    discovery = '/realms/vacation/.well-known/openid-configuration'
    check('Keycloak vacation realm', lambda: request(KEYCLOAK + discovery),
          lambda s, b: s == 200 and json.loads(b)['issuer'] == 'http://localhost:8082/realms/vacation')
    check('Keycloak JWKS',
          lambda: request(KEYCLOAK + '/realms/vacation/protocol/openid-connect/certs'),
          lambda s, b: s == 200 and len(json.loads(b)['keys']) > 0)

    check('Camunda BPMN deployment',
          lambda: request(CAMUNDA + '/process-definition/key/vacation_approval'),
          lambda s, b: s == 200 and json.loads(b)['key'] == 'vacation_approval')
    check('Camunda DMN deployment',
          lambda: request(CAMUNDA + '/decision-definition/key/vacation_decision'),
          lambda s, b: s == 200 and json.loads(b)['key'] == 'vacation_decision')

    query = urllib.parse.quote('SELECT count() FROM vacation.decisions')
    check('ClickHouse decisions table',
          lambda: request(CLICKHOUSE + '/?query=' + query,
                          username=CH_USER, password=CH_PASSWORD),
          lambda s, b: s == 200 and b.strip().isdigit())

    print('READY: infrastructure and access control smoke checks passed.', flush=True)
    print('Complete one employee -> manager -> ClickHouse scenario in the browser.', flush=True)


if __name__ == '__main__':
    try:
        main()
    except Exception as exc:
        print('FAILED:', exc, file=sys.stderr, flush=True)
        sys.exit(1)

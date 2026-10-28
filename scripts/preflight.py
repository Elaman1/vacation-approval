"""Simple no-dependency audit before the one-command Docker build."""
from pathlib import Path
import json
import re
import sys
import xml.etree.ElementTree as ET

root = Path(__file__).resolve().parent.parent
errors = []
warnings = []

required_files = [
    'docker-compose.yml', 'go.mod', 'go.sum',
    'cmd/api/main.go', 'cmd/worker/main.go',
    'cmd/outbox-publisher/main.go', 'cmd/consumer/main.go',
    'frontend/package.json', 'frontend/src/auth.js',
    'frontend/src/api.js', 'frontend/src/App.jsx',
    'infra/keycloak/vacation-realm.json',
    'infra/clickhouse/001_decisions.sql',
    'migrations/001_init.sql', 'migrations/002_workflow.sql',
    'migrations/003_outbox.sql',
    'infra/postgres/02_create_camunda_db.sql',
    'infra/postgres/03_create_keycloak_db.sql',
    'infra/postgres/04_seed_demo_employee.sql',
    'process/vacation.bpmn', 'process/vacation.dmn',
]
for rel in required_files:
    if not (root / rel).is_file():
        errors.append(f'missing: {rel}')

file = root / 'infra/keycloak/vacation-realm.json'
if file.is_file():
    try:
        realm = json.loads(file.read_text(encoding='utf-8'))
        web = next(c for c in realm['clients'] if c.get('clientId') == 'vacation-web')
        scopes = web.get('defaultClientScopes', [])
        if 'basic' not in scopes:
            errors.append('Keycloak vacation-web missing default scope basic (JWT sub => HTTP 401)')
        if 'roles' not in scopes:
            errors.append('Keycloak vacation-web missing default scope roles (JWT realm_access.roles)')
        if 'vacation-api' not in [c.get('clientId') for c in realm['clients']]:
            errors.append('Keycloak vacation-api client missing (JWT audience)')
    except (ValueError, KeyError, StopIteration) as exc:
        errors.append(f'invalid Keycloak realm: {exc}')

client = root / 'internal/integration/camunda/external_task.go'
if client.is_file():
    src = client.read_text(encoding='utf-8')
    start = src.split('func (c *Client) StartVacation', 1)
    if len(start) != 2:
        errors.append('StartVacation missing')
    else:
        start = start[1].split('\nfunc ', 1)[0]
        field = re.search(r'"managerDecision"\s*:\s*map\[string\]any\s*\{([^}]*)\}', start, re.S)
        if not field:
            errors.append('StartVacation missing managerDecision variable map')
        elif not re.search(r'"value"\s*:\s*nil\b', field.group(1)):
            errors.append('StartVacation must set managerDecision to nil, not a preselected enum value (Tasklist Form failure)')

for rel in ['process/vacation.bpmn', 'process/vacation.dmn']:
    try:
        ET.parse(root / rel)
    except (ET.ParseError, FileNotFoundError) as exc:
        errors.append(f'bad XML {rel}: {exc}')

base = root / 'docker-compose.yml'
if base.is_file():
    text = base.read_text(encoding='utf-8')
    for service in ['postgres:', 'camunda:', 'keycloak:', 'kafka:', 'clickhouse:', 'kafka-init:']:
        if service not in text:
            errors.append(f'base docker-compose.yml missing {service}')
    for script in ['002_workflow.sql', '003_outbox.sql', '02_create_camunda_db.sql', '03_create_keycloak_db.sql', '04_seed_demo_employee.sql']:
        if script not in text:
            errors.append(f'base docker-compose.yml missing init mount {script} (clean volumes will fail)')

api = root / 'cmd/api/main.go'
if api.is_file():
    text = api.read_text(encoding='utf-8')
    if 'API_ADDR' not in text:
        errors.append('cmd/api/main.go must honor API_ADDR=:8080 inside Docker')
    if 'OIDC_ISSUER' not in text:
        errors.append('cmd/api/main.go must verify Keycloak JWT (old DevAuth version found)')

print('Vacation Approval preflight')
for warning in warnings:
    print('WARN', warning)
for err in errors:
    print('ERROR', err)
if errors:
    print(f'FAILED: {len(errors)} errors. Fix before docker compose up --build.')
    sys.exit(1)
print(f'OK: {len(required_files)} required project files and critical auth/config checks')

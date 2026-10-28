-- Local demo only. The UUID must equal the imported Keycloak employee user's id.
INSERT INTO employees (keycloak_user_id, balance_days)
VALUES ('11111111-1111-4111-8111-111111111111', 20)
ON CONFLICT (keycloak_user_id) DO NOTHING;

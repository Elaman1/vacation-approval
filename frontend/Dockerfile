# The build context is the repository root, so the nginx config is accessible.
FROM node:22-alpine AS build
WORKDIR /web
COPY frontend/package*.json ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY frontend/ ./
ARG VITE_KEYCLOAK_URL
ARG VITE_KEYCLOAK_REALM=vacation
ARG VITE_KEYCLOAK_CLIENT_ID=vacation-web
ARG VITE_CAMUNDA_TASKLIST_URL
RUN npm run build

FROM nginx:1.28-alpine
COPY infra/nginx/default.conf /etc/nginx/conf.d/default.conf
COPY --from=build /web/dist/ /usr/share/nginx/html/
EXPOSE 80

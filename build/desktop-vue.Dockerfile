FROM nginx:stable-alpine

# Remove default nginx pages
RUN rm -rf /usr/share/nginx/html/*

# Copy all built frontends
COPY frontend-src/desktop/packages/frontend/dist/pwa/ /usr/share/nginx/html/desktop/
COPY frontend-src/settings/packages/frontend/dist/spa/ /usr/share/nginx/html/settings/
COPY frontend-src/login/packages/frontend/dist/spa/ /usr/share/nginx/html/login/
COPY frontend-src/market/frontend/dist/spa/ /usr/share/nginx/html/market/
COPY frontend-src/system-apps/apps/monitoring/dist/pwa/ /usr/share/nginx/html/dashboard/

# Files UI from basic HTML (Go backend serves API, this serves the file browser UI)
COPY frontend/files/ /usr/share/nginx/html/files/

# Nginx config for SPA routing
COPY build/desktop-nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

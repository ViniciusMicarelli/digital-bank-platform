# Build the app, then serve it with nginx, which also proxies the API so the
# browser talks to one origin (no CORS). Context: the repository root
# (the app imports packages/ts-core).
FROM node:24-alpine AS build
WORKDIR /repo/apps/web
COPY apps/web/package.json apps/web/package-lock.json ./
RUN npm ci --ignore-scripts
COPY packages/ts-core /repo/packages/ts-core
COPY apps/web ./
RUN npm run build

FROM nginx:1.29-alpine
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/web/dist /usr/share/nginx/html
EXPOSE 8080

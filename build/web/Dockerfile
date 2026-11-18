FROM node:22-alpine AS dependencies

ARG NPM_REGISTRY=https://registry.npmjs.org
ENV COREPACK_NPM_REGISTRY=${NPM_REGISTRY}
ENV npm_config_registry=${NPM_REGISTRY}
ENV npm_config_update_notifier=false
ENV npm_config_audit=false
ENV npm_config_fund=false
ENV CI=true
RUN npm install -g pnpm@9.15.4 --registry=${NPM_REGISTRY}
WORKDIR /app
COPY package.json pnpm-lock.yaml ./
RUN pnpm --ignore-workspace install --frozen-lockfile --registry=${NPM_REGISTRY}

FROM dependencies AS build

COPY . .
ARG GAMEPANEL_APISERVER_URL=http://apiserver:8080
ENV GAMEPANEL_APISERVER_URL=${GAMEPANEL_APISERVER_URL}
RUN pnpm build

FROM node:22-alpine

WORKDIR /app
ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
ENV GAMEPANEL_APISERVER_URL=http://apiserver:8080
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
COPY --from=build --chown=node:node /app/public ./public
COPY --from=build --chown=node:node /app/ws-proxy-server.mjs ./ws-proxy-server.mjs
USER node
EXPOSE 3000
CMD ["node", "ws-proxy-server.mjs"]

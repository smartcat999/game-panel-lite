FROM node:22-alpine AS dependencies

RUN corepack enable && corepack prepare pnpm@9.15.4 --activate
WORKDIR /app
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

FROM dependencies AS build

COPY . .
ENV GAMEPANEL_CONTROL_PLANE_URL=http://control-plane:8080
ENV GAMEPANEL_REGION_CONTROL_URL=http://region-controller:8081
RUN pnpm build

FROM node:22-alpine

WORKDIR /app
ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
ENV GAMEPANEL_CONTROL_PLANE_URL=http://control-plane:8080
ENV GAMEPANEL_REGION_CONTROL_URL=http://region-controller:8081
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
USER node
CMD ["node", "server.js"]

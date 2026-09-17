# syntax=docker/dockerfile:1.7
FROM node:24-alpine AS build-deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --include=dev --cache /root/.npm --prefer-offline

FROM build-deps AS development
COPY --chown=node:node . .
RUN chown node:node /app /app/node_modules
ENV NODE_ENV=development
USER node
EXPOSE 8080
CMD ["npm", "run", "dev"]

FROM build-deps AS build
ARG HOTKEY_API_ORIGIN=http://backend:8080
ENV HOTKEY_API_ORIGIN=${HOTKEY_API_ORIGIN}
COPY . .
RUN npm run build

FROM node:24-alpine AS production
ENV NODE_ENV=production
ENV PORT=8080
ENV HOSTNAME=0.0.0.0
WORKDIR /app
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
COPY --from=build --chown=node:node /app/public ./public
USER node
EXPOSE 8080
CMD ["node", "server.js"]

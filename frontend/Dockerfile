# =============================================================================
# Stage 1: Dependencies (cached separately)
# =============================================================================
FROM node:latest AS deps

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts

# =============================================================================
# Stage 2: Build
# =============================================================================
FROM node:latest AS builder

WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .

ARG HOTKEY_API_ORIGIN=http://host.docker.internal:8080
ENV HOTKEY_API_ORIGIN=$HOTKEY_API_ORIGIN
ENV NEXT_OUTPUT=standalone

RUN npm run build

# =============================================================================
# Stage 3: Production runner (Next.js standalone)
# =============================================================================
FROM node:latest AS runner

WORKDIR /app

ARG HOTKEY_DEPLOY_ENV=prod

ENV NODE_ENV=production
ENV HOTKEY_DEPLOY_ENV=$HOTKEY_DEPLOY_ENV
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

# Copy standalone server + static assets
COPY --from=builder --chown=node:node /app/.next/standalone ./
COPY --from=builder --chown=node:node /app/.next/static ./.next/static
COPY --from=builder --chown=node:node /app/public ./public

USER node

EXPOSE 3000

CMD ["node", "server.js"]

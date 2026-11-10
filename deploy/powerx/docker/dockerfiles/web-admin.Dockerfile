FROM --platform=$BUILDPLATFORM node:22-alpine3.22 AS builder

WORKDIR /src/web-admin
COPY web-admin/package.json web-admin/package-lock.json ./
RUN npm ci
COPY web-admin/ ./
ARG POWERX_SOURCE_SHA=unknown
ARG POWERX_VERSION=unversioned
ARG POWERX_BUILD_TIME
ARG NUXT_APP_BASE_URL=/
ENV POWERX_BUILD_TARGET=prod POWERX_SOURCE_SHA=$POWERX_SOURCE_SHA POWERX_VERSION=$POWERX_VERSION POWERX_BUILD_TIME=$POWERX_BUILD_TIME NUXT_APP_BASE_URL=$NUXT_APP_BASE_URL
RUN npm run build && node scripts/check-container-output.mjs

FROM node:22-alpine3.22

WORKDIR /app/web-admin
ENV NODE_ENV=production
ENV NITRO_PORT=3000
ENV NITRO_HOST=0.0.0.0

COPY --from=builder /src/web-admin/.output ./.output
COPY --from=builder /src/web-admin/public ./public

EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]

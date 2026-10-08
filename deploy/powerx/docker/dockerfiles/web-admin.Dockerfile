FROM node:22-alpine3.22 AS builder

WORKDIR /src/web-admin
COPY web-admin/package.json web-admin/package-lock.json ./
RUN npm ci
COPY web-admin/ ./
ENV POWERX_BUILD_TARGET=prod
RUN npm run build

FROM node:22-alpine3.22

WORKDIR /app/web-admin
ENV NODE_ENV=production
ENV NITRO_PORT=3000
ENV NITRO_HOST=0.0.0.0

COPY --from=builder /src/web-admin/.output ./.output
COPY --from=builder /src/web-admin/public ./public

EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]

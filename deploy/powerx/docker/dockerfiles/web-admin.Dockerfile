FROM node:20-alpine AS builder

WORKDIR /src/web-admin
COPY web-admin/package.json web-admin/package-lock.json ./
RUN npm ci
COPY web-admin/ ./
RUN npm run build

FROM node:20-alpine

WORKDIR /app/web-admin
ENV NODE_ENV=production
ENV NITRO_PORT=3000
ENV NITRO_HOST=0.0.0.0

COPY --from=builder /src/web-admin/.output ./.output
COPY --from=builder /src/web-admin/public ./public

EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]

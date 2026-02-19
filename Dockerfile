FROM node:lts-alpine AS builder

WORKDIR /app

COPY package*.json ./

RUN --mount=type=cache,target=/root/.npm npm ci

COPY . .

RUN npm run build

FROM node:lts-alpine AS release

RUN apk update && apk upgrade && apk add --no-cache dumb-init

WORKDIR /app

COPY --from=builder /app/dist /app/dist
COPY --from=builder /app/package.json /app/package.json
COPY --from=builder /app/package-lock.json /app/package-lock.json

ENV NODE_ENV=production

RUN npm ci --ignore-scripts --omit-dev

# Use dumb-init to handle signals properly
ENTRYPOINT ["dumb-init", "--", "node", "dist/index.js"]

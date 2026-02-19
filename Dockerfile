FROM node:lts-alpine AS builder

WORKDIR /app

# Copy package files and install dependencies
# Using npm install instead of npm ci to handle package.json changes
COPY package*.json ./

RUN --mount=type=cache,target=/root/.npm npm install

COPY . .

RUN npm run build

FROM node:lts-alpine AS release

WORKDIR /app

# Install dumb-init for proper signal handling
RUN apk update && apk upgrade && apk add --no-cache dumb-init

# Copy built artifacts and package files
COPY --from=builder /app/dist /app/dist
COPY --from=builder /app/package.json /app/package.json
COPY --from=builder /app/package-lock.json /app/package-lock.json

ENV NODE_ENV=production

# Install production dependencies only
RUN npm ci --ignore-scripts --omit-dev

# Use dumb-init to handle signals properly
ENTRYPOINT ["dumb-init", "--", "node", "dist/index.js"]

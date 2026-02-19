FROM node:lts-alpine AS builder

WORKDIR /app

# Copy package files and install dependencies
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

ENV NODE_ENV=production

# Install production dependencies only
RUN npm install --omit=dev --ignore-scripts

# Use dumb-init to handle signals properly
ENTRYPOINT ["dumb-init", "--", "node", "dist/index.js"]

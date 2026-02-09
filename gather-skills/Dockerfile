FROM node:20-alpine

WORKDIR /app

# Install dependencies
COPY package*.json ./
RUN npm ci --only=production

# Copy built files and assets
COPY dist/ ./dist/
COPY src/views/ ./dist/views/
COPY skills/ ./skills/

# Create data directory
RUN mkdir -p /app/data

ENV NODE_ENV=production
ENV PORT=3000
ENV DATABASE_PATH=/app/data/reskill.db

EXPOSE 3000

CMD ["node", "dist/api/index.js"]

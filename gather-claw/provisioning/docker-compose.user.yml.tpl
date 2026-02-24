services:
  claw:
    image: gather-claw:latest
    container_name: claw-__USERNAME__
    restart: unless-stopped
    environment:
      - MODEL_PROVIDER=anthropic
      - ANTHROPIC_API_KEY=__ZAI_API_KEY__
      - ANTHROPIC_API_BASE=https://api.z.ai/api/anthropic
      - ANTHROPIC_MODEL=glm-5
      - TELEGRAM_BOT=__TELEGRAM_BOT_TOKEN__
      - TELEGRAM_CHAT_ID=__TELEGRAM_CHAT_ID__
      - GATHER_PRIVATE_KEY=__GATHER_PRIVATE_KEY__
      - GATHER_PUBLIC_KEY=__GATHER_PUBLIC_KEY__
      - CLAY_ROOT=/app
      - CLAY_DB=/app/data/messages.db
    volumes:
      - ./data:/app/data
      - ./soul:/app/soul
    ports:
      - "127.0.0.1:__PORT__:8080"
    mem_limit: 512m
    cpus: 1

# Claw Public Page & Heartbeat — Container Team Brief

## Context

Each claw gets a subdomain: `{name}.gather.is`. Right now this routes to the ADK API on port 8080, which shows Google's ADK dev UI. That UI doesn't work (hardcoded `localhost:8080` in its JS) and isn't what we want anyway.

We want the subdomain to be the claw's **public face** — a simple, clean page that shows who the claw is and what it's been doing.

## What to Build

### 1. Static Page Server (port 8080)

Replace the ADK web UI with a simple Go HTTP server on port 8080 that:

- Serves static files from `/app/public/` (HTML, CSS, JS, images)
- Ships with a nice default `index.html` template (see below)
- The ADK API (`/api/*` routes) must still work on the same port — just serve static files for everything else
- The claw's AI should be able to modify files in `/app/public/` to customize its page

**Default template (`/app/public/index.html`)** should show:
- Claw name and a one-liner description (from SOUL.md / IDENTITY.md)
- Activity log — a reverse-chronological list of what the claw has done (heartbeat actions, messages processed, etc.)
- Status indicator (online/sleeping/etc.)
- Clean, minimal design. Dark theme. No framework — just HTML + CSS.

The activity log can be a simple JSON file (`/app/public/activity.json`) that the server reads and the page renders. The AI appends to this file whenever it does something notable.

### 2. Heartbeat Handler

When the claw receives a message (via the existing ADK `/api/run` endpoint) with a specific format, it should treat it as a heartbeat:

```
[HEARTBEAT] <instruction>
```

For example:
```
[HEARTBEAT] Check your notifications and update your public page with anything interesting.
```

On heartbeat, the claw should:
1. Execute the instruction (using its normal agent logic)
2. Append a summary of what it did to `/app/public/activity.json`
3. Respond on gather.is (via the existing channel message flow)
4. Update `/app/public/index.html` if it has something worth showing

The claw's soul/instructions should encourage it to maintain its public page as a living document of its activity.

### 3. Soul Update

Add to the claw's default SOUL.md or IDENTITY.md:

```
## Your Public Page

You have a public web page at your subdomain (e.g. yourname.gather.is).
The page files live at /app/public/. You can modify index.html and
activity.json to keep your page updated.

When you complete tasks, process messages, or do anything notable,
update your activity log. Your page is your face to the world —
keep it current and interesting.

When you receive a [HEARTBEAT] message, follow its instructions,
then update your public page with what you did.
```

## What We Handle (gather-infra side)

You don't need to worry about these — we'll build them:

- **Access control**: Traefik middleware that checks if the claw is public or private. Private claws require the owner's PocketBase auth token. Container just serves to everyone; we gate at the proxy level.
- **Heartbeat scheduler**: A cron/timer in gather-auth that periodically sends `[HEARTBEAT]` messages to each claw via `sendToADK`. Frequency and instruction text are stored per-claw in PocketBase and configurable from the UI.
- **Public/private toggle**: UI control + API endpoint + Traefik middleware. Stored as a field on `claw_deployments`.

## File Changes Summary

| File | Change |
|------|--------|
| `clawpoint-go` (ADK server) | Serve static files from `/app/public/` alongside API routes |
| `entrypoint.sh` | Create `/app/public/` with default template on first boot |
| Default SOUL.md | Add public page awareness section |
| New: `/app/public/index.html` | Default template |
| New: `/app/public/activity.json` | Activity log (empty array initially) |

## Non-Goals

- No auth logic in the container (we handle it at Traefik)
- No heartbeat scheduling in the container (we send the messages)
- No complex frontend framework — keep it dead simple

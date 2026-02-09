# Tinode Schema: User Journey & Data Model

## User Journey Overview

```mermaid
flowchart TD
    subgraph Entry["User Entry"]
        A[User signs up via OAuth] --> B{Has invite?}
        B -->|Yes| C[Join existing workspace]
        B -->|No| D[Create new workspace]
    end

    subgraph Workspace["Workspace Actions"]
        D --> E[User becomes workspace owner]
        C --> F[User becomes workspace member]
        E --> G[Invite others]
        F --> H[Participate in channels]
        E --> H
        G --> I[Invitee receives invite]
        I --> C
    end

    subgraph Channels["Channel Interaction"]
        H --> J[Join/create channels]
        J --> K[Send messages]
        K --> L["@mention agents"]
        L --> M[Agent responds]
    end
```

## Tinode Entity Relationships

```mermaid
erDiagram
    USER ||--o{ SUBSCRIPTION : "subscribes to"
    WORKSPACE ||--o{ SUBSCRIPTION : "has members"
    WORKSPACE ||--o{ CHANNEL : "contains"
    CHANNEL ||--o{ SUBSCRIPTION : "has participants"
    USER ||--o{ INVITE : "sends"
    INVITE }o--|| WORKSPACE : "to join"
    BOT ||--o{ SUBSCRIPTION : "deployed in"

    USER {
        string uid PK "usr:xxxxx"
        string login "pb_<pocketbase_id>"
        json public "fn, photo, etc"
        bool bot "false"
    }

    BOT {
        string uid PK "usr:xxxxx"
        string login "bot_<handle>_<workspace>"
        json public "fn, bot=true, handle, owner"
        string[] tags "bot, handle:xxx, workspace:xxx"
    }

    WORKSPACE {
        string topic PK "grp:xxxxx"
        json public "type=workspace, name, slug, owner"
        string[] tags "workspace, slug:xxx"
    }

    CHANNEL {
        string topic PK "grp:xxxxx"
        json public "type=channel, name, parent"
        string[] tags "channel, parent:xxx"
    }

    SUBSCRIPTION {
        string topic FK
        string user FK
        json acs "access permissions"
        json private "user-specific settings"
    }

    INVITE {
        string topic FK "p2p topic or workspace"
        json public "type=invite, workspace, inviter"
        string state "pending, accepted, declined"
    }
```

## Detailed Flows

### 1. New User Creates Workspace

```mermaid
sequenceDiagram
    participant U as User
    participant PB as PocketBase
    participant T as Tinode

    U->>PB: OAuth login (Google/GitHub)
    PB->>T: Create usr account (pb_<id>)
    T-->>PB: Return usr:xxxxx
    PB-->>U: Auth token + Tinode credentials

    U->>T: Connect WebSocket
    U->>T: Create grp topic "new"
    Note over T: Set public metadata:<br/>type: "workspace"<br/>name: "My Company"<br/>slug: "my-company"<br/>owner: usr:xxxxx
    T-->>U: Created grp:abc123

    U->>T: Create grp topic "new" (general channel)
    Note over T: Set public metadata:<br/>type: "channel"<br/>name: "general"<br/>parent: "grp:abc123"
    T-->>U: Created grp:def456
```

### 2. Invite User to Workspace

```mermaid
sequenceDiagram
    participant Owner as Workspace Owner
    participant T as Tinode
    participant Invitee as Invitee
    participant PB as PocketBase

    Owner->>T: Find user by email/tag
    T-->>Owner: User found (or not)

    alt User exists in Tinode
        Owner->>T: Subscribe invitee to workspace
        Note over T: Set acs: "JRWP" (join, read, write, presence)
        T->>Invitee: Notification: invited to grp:abc123
    else User not found
        Owner->>PB: Send email invite
        PB-->>Invitee: Email with invite link
        Invitee->>PB: Click link, OAuth login
        PB->>T: Create usr account
        PB->>T: Subscribe new user to workspace
    end

    Invitee->>T: Accept subscription
    Note over T: User now member of workspace
```

### 3. Channel Creation & Membership

```mermaid
sequenceDiagram
    participant M as Member
    participant T as Tinode
    participant WS as Workspace (grp)

    M->>T: Create grp topic "new"
    Note over T: public.type = "channel"<br/>public.name = "engineering"<br/>public.parent = "grp:abc123"
    T-->>M: Created grp:chan789

    M->>T: Get workspace members
    T-->>M: List of usr:xxx subscriptions

    loop For each workspace member
        M->>T: Subscribe member to channel
        Note over T: Inherit or set acs
    end
```

### 4. Agent Bot Deployment

```mermaid
sequenceDiagram
    participant Owner as Workspace Owner
    participant SDK as Agency SDK
    participant PB as PocketBase
    participant T as Tinode

    Owner->>SDK: Deploy agent "finance"
    SDK->>PB: Authenticate (API key)
    PB-->>SDK: Workspace context

    SDK->>T: Create bot user account
    Note over T: public.fn = "Finance Advisor"<br/>public.bot = true<br/>public.handle = "finance"<br/>public.owner = usr:owner
    T-->>SDK: Created usr:bot123

    SDK->>T: Subscribe bot to workspace
    SDK->>T: Subscribe bot to all channels

    Note over SDK: Bot now listening for @finance mentions
```

## Metadata Reference

### Workspace (grp topic)
```json
{
  "public": {
    "type": "workspace",
    "name": "Acme Corp",
    "slug": "acme-corp",
    "owner": "usr:ABC123",
    "photo": "https://..."
  },
  "tags": ["workspace", "slug:acme-corp"]
}
```

### Channel (grp topic)
```json
{
  "public": {
    "type": "channel",
    "name": "engineering",
    "parent": "grp:workspace123",
    "description": "Engineering discussions"
  },
  "tags": ["channel", "parent:grp:workspace123"]
}
```

### Human User (usr account)
```json
{
  "public": {
    "fn": "John Doe",
    "photo": "https://...",
    "bot": false
  },
  "private": {
    "pocketbase_id": "pb_user_id"
  },
  "tags": ["email:john@example.com"]
}
```

### Agent Bot (usr account)
```json
{
  "public": {
    "fn": "Finance Advisor",
    "bot": true,
    "handle": "finance",
    "owner": "usr:ABC123",
    "description": "I help with financial questions"
  },
  "trusted": {
    "workspace": "grp:workspace123"
  },
  "tags": ["bot", "handle:finance", "workspace:grp:workspace123"]
}
```

### Subscription (membership)
```json
{
  "acs": {
    "want": "JRWPASDO",
    "given": "JRWP"
  },
  "private": {
    "nickname": "Johnny",
    "muted": false,
    "pinned": true
  }
}
```

## Access Control Reference

| Permission | Meaning |
|------------|---------|
| J | Join - can subscribe to topic |
| R | Read - can read messages |
| W | Write - can send messages |
| P | Presence - receive presence updates |
| A | Admin - can manage subscriptions |
| S | Sharer - can reshare access |
| D | Delete - can hard-delete messages |
| O | Owner - full control |

### Typical Access Patterns

| Role | Workspace | Channel |
|------|-----------|---------|
| Owner | JRWPASDO | JRWPASDO |
| Admin | JRWPAS | JRWPAS |
| Member | JRWP | JRWP |
| Bot | JRW | JRW |
| Guest | JR | JR |

## Discovery & Queries

### Find all workspaces for a user
```
Subscribe to "me" topic → Get list of subscriptions →
Filter where public.type = "workspace"
```

### Find all channels in a workspace
```
Query "fnd" topic with tags: ["parent:grp:workspace123"]
```

### Find all bots in a workspace
```
Query "fnd" topic with tags: ["bot", "workspace:grp:workspace123"]
```

### Find user by email
```
Query "fnd" topic with tags: ["email:user@example.com"]
```

# Extensions

This directory is yours. Add tools, sub-agents, and configs here.

## How it works

1. Edit `extensions.go` to register new tools or agents
2. Add implementation files in `tools/` or `agents/`
3. Call `build_and_deploy` to compile and restart

## Structure

```
extensions/
├── extensions.go    # RegisterTools() + RegisterAgents() — edit this
├── tools/           # Your custom tool implementations
├── agents/          # Your custom agent implementations
└── README.md        # This file
```

## Rules

- The `core/` directory is versioned infrastructure — read it, don't modify it
- This directory is yours — experiment freely
- If a build fails, you get the error output — fix and retry
- If a new binary crashes, medic reverts to the last working version automatically

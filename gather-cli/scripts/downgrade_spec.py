#!/usr/bin/env python3
"""Downgrade an OpenAPI 3.1 spec to 3.0.3 for oapi-codegen compatibility.

Handles:
- type: [X, "null"] → type: X + nullable: true
- Removes $schema properties from component schemas (3.1-only)
- Sets openapi version to 3.0.3
"""
import json
import sys


def fix_nullable(obj):
    """Recursively convert 3.1 nullable syntax to 3.0."""
    if isinstance(obj, dict):
        if "type" in obj and isinstance(obj["type"], list):
            types = [t for t in obj["type"] if t != "null"]
            if len(types) == 1:
                obj["type"] = types[0]
                obj["nullable"] = True
        for v in obj.values():
            fix_nullable(v)
    elif isinstance(obj, list):
        for item in obj:
            fix_nullable(item)


def main():
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <input.json> <output.json>", file=sys.stderr)
        sys.exit(1)

    with open(sys.argv[1]) as f:
        spec = json.load(f)

    fix_nullable(spec)
    spec["openapi"] = "3.0.3"

    # Remove $schema properties (3.1-only feature)
    for schema in spec.get("components", {}).get("schemas", {}).values():
        props = schema.get("properties", {})
        if "$schema" in props:
            del props["$schema"]
            req = schema.get("required", [])
            if "$schema" in req:
                req.remove("$schema")

    with open(sys.argv[2], "w") as f:
        json.dump(spec, f, indent=2)


if __name__ == "__main__":
    main()

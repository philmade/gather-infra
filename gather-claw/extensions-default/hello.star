# DESCRIPTION: Example extension — greet someone and demonstrate Starlark builtins
#
# This is a Starlark script — a Python dialect that runs embedded in ClawPoint-Go.
# It runs instantly, no compilation needed. Scripts in /app/data/extensions/ persist
# across restarts.
#
# Usage: extension_run(name="hello", args={"name": "WebClawMan"})
#
# Available builtins:
#   http_get(url)                  — GET a URL, returns response body
#   http_post(url, body, type)     — POST to a URL (default type: application/json)
#   read_file(path)                — read a file from disk
#   write_file(path, content)      — write a file to disk
#   log(msg)                       — output a message (captured in results)

def run(args):
    name = args.get("name", "world")
    log("Running hello extension...")
    return "Hello, " + name + "! I'm a Starlark extension running inside ClawPoint-Go."

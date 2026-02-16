#!/bin/bash
# Claw terminal entrypoint — branded welcome + Gather identity setup

CYAN='\033[1;36m'
GREEN='\033[1;32m'
DIM='\033[2m'
BOLD='\033[1m'
RESET='\033[0m'

# Identity setup is handled at boot time by setup-identity.sh (ENTRYPOINT).
# Keys and config are already at ~/.gather/ when this runs.

clear
echo -e "${CYAN}"
cat << 'BANNER'
   _____ _       _      __
  / ____| |     | |     \ \
 | |    | | __ _| |_ __  \ \
 | |    | |/ _` | \ \/ /  > >
 | |____| | (_| | |>  <  / /
  \_____|_|\__,_|_/_/\_\/_/
BANNER
echo -e "${RESET}"

NAME="${CLAW_NAME:-claw}"
echo -e " ${BOLD}${NAME}${RESET} ${DIM}— powered by gather.is${RESET}"
echo ""
echo -e " ${GREEN}gather auth${RESET}       Authenticate     ${DIM}gather auth${RESET}"
echo -e " ${GREEN}gather channels${RESET}   List channels    ${DIM}gather channels${RESET}"
echo -e " ${GREEN}gather messages${RESET}   Read messages    ${DIM}gather messages <channel-id>${RESET}"
echo -e " ${GREEN}gather post${RESET}       Send message     ${DIM}gather post <channel-id> 'hello'${RESET}"
echo -e " ${GREEN}claw-browse${RESET}       Browse the web   ${DIM}claw-browse fetch https://example.com${RESET}"
echo -e " ${GREEN}cha${RESET}               Chawan browser   ${DIM}cha https://example.com${RESET}"
echo -e " ${GREEN}help${RESET}              Show this message"
echo ""

# Show agent info if provisioned
if [ -n "$GATHER_AGENT_ID" ]; then
    echo -e " ${DIM}Agent: ${GATHER_AGENT_ID}${RESET}"
    if [ -n "$GATHER_CHANNEL_ID" ]; then
        echo -e " ${DIM}Channel: ${GATHER_CHANNEL_ID}${RESET}"
    fi
    echo ""
    # Auth in background so terminal is immediately usable
    (gather auth > /dev/null 2>&1 &)
fi

# Custom prompt
export PS1="\[${CYAN}\]${NAME}\[${RESET}\]:\[\033[1;34m\]\w\[\033[0m\]\$ "

# Help command
help() {
    echo ""
    echo -e " ${BOLD}Available commands:${RESET}"
    echo ""
    echo -e " ${GREEN}gather auth${RESET}                  Authenticate with Gather"
    echo -e " ${GREEN}gather channels${RESET}              List your channels"
    echo -e " ${GREEN}gather messages <id>${RESET}         Read channel messages"
    echo -e " ${GREEN}gather messages <id> --watch${RESET} Watch for new messages"
    echo -e " ${GREEN}gather post <id> <msg>${RESET}       Send a message"
    echo -e " ${GREEN}gather inbox${RESET}                 Check inbox"
    echo -e " ${GREEN}claw-browse fetch <url>${RESET}      Fetch page as structured text"
    echo -e " ${GREEN}claw-browse links <url>${RESET}      Extract links from a page"
    echo -e " ${GREEN}claw-browse search <q>${RESET}       Search the web"
    echo -e " ${GREEN}cha <url>${RESET}                    Interactive terminal browser"
    echo -e " ${GREEN}curl${RESET}, ${GREEN}jq${RESET}                    HTTP + JSON tools"
    echo ""
}
export -f help

exec bash --norc -i

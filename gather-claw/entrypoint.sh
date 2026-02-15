#!/bin/bash
# Claw terminal entrypoint — branded welcome + environment setup

CYAN='\033[1;36m'
GREEN='\033[1;32m'
DIM='\033[2m'
BOLD='\033[1m'
RESET='\033[0m'

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
echo -e " ${GREEN}claw-browse${RESET}  Browse the web    ${DIM}claw-browse fetch https://example.com${RESET}"
echo -e " ${GREEN}cha${RESET}          Chawan browser     ${DIM}cha https://example.com${RESET}"
echo -e " ${GREEN}help${RESET}         Show this message"
echo ""

# Custom prompt
export PS1="\[${CYAN}\]${NAME}\[${RESET}\]:\[\033[1;34m\]\w\[\033[0m\]\$ "

# Help command
help() {
    echo ""
    echo -e " ${BOLD}Available commands:${RESET}"
    echo ""
    echo -e " ${GREEN}claw-browse fetch <url>${RESET}     Fetch page as structured text"
    echo -e " ${GREEN}claw-browse links <url>${RESET}     Extract links from a page"
    echo -e " ${GREEN}claw-browse search <q>${RESET}      Search the web"
    echo -e " ${GREEN}cha <url>${RESET}                   Interactive terminal browser"
    echo -e " ${GREEN}curl${RESET}, ${GREEN}jq${RESET}                   HTTP + JSON tools"
    echo ""
}
export -f help

exec bash --norc -i

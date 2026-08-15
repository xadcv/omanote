#!/usr/bin/env bash
set -euo pipefail

REPO="github.com/xadcv/omanote"
PLUGIN_URL="https://github.com/xadcv/omanote.git"
BIN_NAME="omanote"

log() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }

has_cmd() { command -v "$1" >/dev/null 2>&1; }

append_line_if_missing() {
    local line="$1" file="$2"
    mkdir -p "$(dirname "$file")"
    touch "$file"
    if ! grep -Fqx "$line" "$file"; then
        printf '\n%s\n' "$line" >>"$file"
        return 0
    fi
    return 1
}

add_to_path_for_mise() {
    has_cmd mise || return 0

    local sh="${SHELL##*/}"
    local rc_file
    case "$sh" in
        zsh) rc_file="$HOME/.zshrc" ;;
        fish) rc_file="$HOME/.config/fish/config.fish" ;;
        *) rc_file="$HOME/.bashrc" ;;
    esac

    local path_line='export PATH="$HOME/go/bin:$PATH"'
    if append_line_if_missing "$path_line" "$rc_file"; then
        log "Detected mise. Added $HOME/go/bin to PATH in $rc_file"
    else
        log "Detected mise. PATH already includes $HOME/go/bin in $rc_file"
    fi
}

setup_omarchy_plugin() {
    has_cmd omarchy || return 0
    has_cmd omarchy-shell || return 0

    printf '\nDetected Omarchy Quattro.\n'
    read -r -p "Add Omanote as a Quattro shell plugin? [y/N] " ans
    case "${ans,,}" in
        y|yes)
            if omarchy plugin add "$PLUGIN_URL" --enable --yes; then
                log "Enabled Quattro plugin xadcv.omanote."
                log "Summon it with: omarchy-shell shell summon xadcv.omanote '{}'"
                log "Or run: omanote menu"
            else
                warn "Could not add the Quattro plugin. Add it later with:"
                warn "  omarchy plugin add $PLUGIN_URL --enable"
            fi
            ;;
        *)
            log "Skipped Quattro plugin setup."
            log "Add it later with: omarchy plugin add $PLUGIN_URL --enable"
            ;;
    esac
}

setup_omarchy_binding() {
    has_cmd hyprctl || return 0
    has_cmd omarchy-shell || return 0

    local bind_line='bindd = SUPER SHIFT, R, Omanote, exec, omanote menu'
    local hypr_conf="$HOME/.config/hypr/hyprland.conf"

    printf '\nDetected Hyprland + Omarchy Quattro.\n'
    read -r -p "Add SUPER+SHIFT+R Omanote panel keybind to $hypr_conf? [y/N] " ans
    case "${ans,,}" in
        y|yes)
            if append_line_if_missing "$bind_line" "$hypr_conf"; then
                log "Added Hyprland bind: $bind_line"
            else
                log "Hyprland bind already exists."
            fi
            ;;
        *)
            log "Skipped Hyprland keybinding setup."
            ;;
    esac
}

main() {
    if ! has_cmd go; then
        warn "go is required. Install Go 1.25.5+ and retry."
        exit 1
    fi

    log "Installing $BIN_NAME from $REPO ..."
    go install "$REPO@latest"
    log "Installed $BIN_NAME to \\$(go env GOPATH)/bin"

    add_to_path_for_mise
    setup_omarchy_plugin
    setup_omarchy_binding

    log "Done. Run '$BIN_NAME' to open the TUI."
}

main "$@"

#!/usr/bin/env bash
set -euo pipefail

REPO="github.com/xadcv/omanote"
PLUGIN_URL="https://github.com/xadcv/omanote.git"
PLUGIN_ID="xadcv.omanote"
BIN_NAME="omanote"
INSTALL_DIR="${HOME}/.local/bin"
INSTALL_BIN="${INSTALL_DIR}/${BIN_NAME}"
ASSUME_YES=0

log() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }

has_cmd() { command -v "$1" >/dev/null 2>&1; }

confirm() {
    local prompt="$1" ans
    if ((ASSUME_YES)); then
        return 0
    fi
    if [[ ! -r /dev/tty || ! -w /dev/tty ]]; then
        warn "$prompt skipped because no interactive terminal is available."
        return 1
    fi
    read -r -p "$prompt [y/N] " ans </dev/tty
    case "${ans,,}" in
        y|yes) return 0 ;;
        *) return 1 ;;
    esac
}

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

ensure_install_dir_on_path() {
    case ":${PATH}:" in
        *":${INSTALL_DIR}:"*) return 0 ;;
    esac
    local sh="${SHELL##*/}"
    local rc_file path_line
    case "$sh" in
        zsh)
            rc_file="$HOME/.zshrc"
            path_line='export PATH="$HOME/.local/bin:$PATH"'
            ;;
        fish)
            rc_file="$HOME/.config/fish/config.fish"
            path_line='fish_add_path "$HOME/.local/bin"'
            ;;
        *)
            rc_file="$HOME/.bashrc"
            path_line='export PATH="$HOME/.local/bin:$PATH"'
            ;;
    esac
    if append_line_if_missing "$path_line" "$rc_file"; then
        log "Added $INSTALL_DIR to PATH in $rc_file"
    else
        log "$INSTALL_DIR is already configured in $rc_file"
    fi
    export PATH="$INSTALL_DIR:$PATH"
}

setup_omarchy_plugin() {
    has_cmd omarchy || return 0
    has_cmd omarchy-shell || return 0

    printf '\nDetected Omarchy Quattro.\n'
    if ! confirm "Add Omanote as a Quattro shell plugin?"; then
        log "Skipped Quattro plugin setup."
        log "Add it later with: omarchy plugin add $PLUGIN_URL --enable"
        return 0
    fi

    if has_cmd jq && omarchy plugin list --json | jq -e --arg id "$PLUGIN_ID" 'any(.[]; .id == $id)' >/dev/null; then
        omarchy plugin update "$PLUGIN_ID" --yes || warn "Could not update existing plugin $PLUGIN_ID."
        if ! omarchy plugin enable "$PLUGIN_ID" --section right; then
            warn "Could not enable existing plugin $PLUGIN_ID."
            return 0
        fi
    elif ! omarchy plugin add "$PLUGIN_URL" --enable --yes; then
        warn "Could not add the Quattro plugin. Add it later with:"
        warn "  omarchy plugin add $PLUGIN_URL --enable"
        return 0
    fi

    log "Enabled Quattro plugin $PLUGIN_ID."
    log "Summon it with: omarchy-shell shell summon $PLUGIN_ID '{}'"
    log "Or run: omanote menu"
}

setup_omarchy_binding() {
    has_cmd hyprctl || return 0
    has_cmd omarchy-shell || return 0

    local bind_line='o.bind("SUPER + SHIFT + R", "Omanote Menu", "omanote menu")'
    local unbind_line='hl.unbind("SUPER + SHIFT + R")'
    local bindings_file="$HOME/.config/hypr/bindings.lua"
    local current_binding=""

    printf '\nDetected Hyprland + Omarchy Quattro.\n'
    if has_cmd omarchy; then
        current_binding="$(omarchy menu keybindings --print 2>/dev/null | grep -F -m1 'SUPER SHIFT + R' || true)"
    fi
    if [[ "$current_binding" == *"Omanote"* ]]; then
        log "SUPER+SHIFT+R is already bound to Omanote."
        return 0
    fi
    if ! confirm "Bind SUPER+SHIFT+R to the Omanote panel in $bindings_file?"; then
        log "Skipped Hyprland keybinding setup."
        return 0
    fi

    if [[ -n "$current_binding" ]]; then
        log "SUPER+SHIFT+R was previously bound as: $current_binding"
        append_line_if_missing "$unbind_line" "$bindings_file" || true
    fi
    if append_line_if_missing "$bind_line" "$bindings_file"; then
        log "Added Hyprland bind: $bind_line"
    else
        log "Hyprland bind already exists in $bindings_file."
    fi

    if ! hyprctl reload >/dev/null; then
        warn "Hyprland could not reload the updated binding."
        return 0
    fi
    local config_errors
    config_errors="$(hyprctl configerrors 2>&1 || true)"
    if [[ -n "$config_errors" ]]; then
        warn "Hyprland reports configuration errors:"
        warn "$config_errors"
    fi
}

setup_service() {
    has_cmd systemctl || return 0

    printf '\nOmanote can keep its controller available as a user service.\n'
    if ! confirm "Enable and start omanote.service?"; then
        log "Skipped service setup."
        log "Enable it later with: omanote autostart enable"
        return 0
    fi

    if ! "$INSTALL_BIN" autostart enable; then
        warn "Could not install omanote.service."
        return 0
    fi
    if systemctl --user start omanote.service; then
        log "Enabled and started omanote.service."
    else
        warn "The service was enabled but could not be started now."
    fi
}

main() {
    while (($#)); do
        case "$1" in
            -y|--yes) ASSUME_YES=1 ;;
            -h|--help)
                log "Usage: install.sh [--yes]"
                return 0
                ;;
            *)
                warn "unknown option: $1"
                return 1
                ;;
        esac
        shift
    done

    if ! has_cmd go; then
        warn "go is required. Install Go 1.25.5+ and retry."
        exit 1
    fi

    log "Installing $BIN_NAME from $REPO ..."
    mkdir -p "$INSTALL_DIR"
    GOBIN="$INSTALL_DIR" go install "$REPO@latest"
    log "Installed $BIN_NAME to $INSTALL_BIN"

    ensure_install_dir_on_path
    setup_service
    setup_omarchy_plugin
    setup_omarchy_binding

    log "Done. Run '$BIN_NAME' to open the TUI."
}

main "$@"

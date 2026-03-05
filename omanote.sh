#!/usr/bin/env bash
set -euo pipefail

CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/omanote"
MODULES_FILE="$CACHE_DIR/modules"
SINK_NAME="OmanoteMix"
SOURCE_NAME="Omanote"

mkdir -p "$CACHE_DIR"

die() { echo "error: $*" >&2; exit 1; }

is_running() {
    [[ -f "$MODULES_FILE" ]] || return 1
    local first_mod
    first_mod=$(head -1 "$MODULES_FILE")
    pactl list modules short | grep -q "^${first_mod}[[:space:]]"
}

get_default_sink() {
    pactl get-default-sink
}

get_default_source() {
    pactl list sources short | grep -v '\.monitor' | grep -v "$SINK_NAME" | grep -v "$SOURCE_NAME" | head -1 | cut -f2
}

cmd_start() {
    if is_running 2>/dev/null; then
        echo "Already running. Use 'omanote stop' first."
        exit 0
    fi

    rm -f "$MODULES_FILE"

    local default_sink default_source sink_mod remap_mod mic_mod sys_mod

    default_sink=$(get_default_sink) || die "No default sink found"
    default_source=$(get_default_source) || die "No microphone found"

    echo "Default speaker: $default_sink"
    echo "Default mic:     $default_source"

    # 1. Create null sink (the mixing point)
    sink_mod=$(pactl load-module module-null-sink \
        sink_name="$SINK_NAME" \
        "sink_properties=device.description=$SINK_NAME" \
        channel_map=stereo)
    echo "Created null sink: $SINK_NAME (module $sink_mod)"

    # 2. Create remap-source so "Omanote" appears as a selectable mic
    remap_mod=$(pactl load-module module-remap-source \
        source_name="$SOURCE_NAME" \
        "master=${SINK_NAME}.monitor" \
        "source_properties=device.description=$SOURCE_NAME")
    echo "Created remap source: $SOURCE_NAME (module $remap_mod)"

    # 3. Loopback: physical mic → virtual sink
    mic_mod=$(pactl load-module module-loopback \
        source="$default_source" \
        sink="$SINK_NAME" \
        latency_msec=20)
    echo "Created mic loopback (module $mic_mod)"

    # 4. Loopback: system audio monitor → virtual sink
    sys_mod=$(pactl load-module module-loopback \
        source="${default_sink}.monitor" \
        sink="$SINK_NAME" \
        latency_msec=20)
    echo "Created sys loopback (module $sys_mod)"

    # Persist all module IDs
    printf '%s\n%s\n%s\n%s\n' "$sink_mod" "$remap_mod" "$mic_mod" "$sys_mod" > "$MODULES_FILE"

    echo ""
    echo "Omanote active"
    echo "Select \"$SOURCE_NAME\" as your microphone in browser/app settings."
}

cmd_stop() {
    if [[ ! -f "$MODULES_FILE" ]]; then
        echo "Not running."
        return
    fi

    # Unload in reverse order
    tac "$MODULES_FILE" | while read -r mod_id; do
        mod_id=$(echo "$mod_id" | tr -d '[:space:]')
        [[ -z "$mod_id" ]] && continue
        if pactl unload-module "$mod_id" 2>/dev/null; then
            echo "Unloaded module $mod_id"
        fi
    done

    rm -f "$MODULES_FILE"
    echo "Omanote stopped."
}

cmd_status() {
    if is_running 2>/dev/null; then
        echo "Running"
        echo "  Modules: $(cat "$MODULES_FILE" | tr '\n' ' ')"
        echo ""
        echo "Sources:"
        pactl list sources short | grep -E "$SINK_NAME|$SOURCE_NAME|$(get_default_source 2>/dev/null || true)" || true
    else
        rm -f "$MODULES_FILE"
        echo "Not running."
    fi
}

case "${1:-}" in
    start)  cmd_start ;;
    stop)   cmd_stop ;;
    status) cmd_status ;;
    *)
        echo "Usage: omanote {start|stop|status}"
        echo ""
        echo "  start   Create Omanote virtual mic (mic + system audio)"
        echo "  stop    Remove Omanote virtual mic"
        echo "  status  Show current state"
        ;;
esac

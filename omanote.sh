#!/usr/bin/env bash
set -euo pipefail

CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/omanote"
PID_FILE="$CACHE_DIR/pids"
MODULE_FILE="$CACHE_DIR/module_id"
REMAP_MODULE_FILE="$CACHE_DIR/remap_module_id"
SINK_NAME="OmanoteMix"
SOURCE_NAME="Omanote"

mkdir -p "$CACHE_DIR"

die() { echo "error: $*" >&2; exit 1; }

is_running() { [[ -f "$PID_FILE" ]] && kill -0 $(cat "$PID_FILE" | tr '\n' ' ') 2>/dev/null; }

get_default_sink() {
    pactl get-default-sink
}

get_default_source() {
    # Prefer a hardware mic, skip monitors and our own virtual devices
    pactl list sources short | grep -v '\.monitor' | grep -v "$SINK_NAME" | grep -v "$SOURCE_NAME" | head -1 | cut -f2
}

cmd_start() {
    if is_running 2>/dev/null; then
        echo "Already running. Use 'omanote stop' first."
        exit 0
    fi

    # Clean up stale state
    rm -f "$PID_FILE" "$MODULE_FILE" "$REMAP_MODULE_FILE"

    local default_sink default_source module_id remap_module_id

    default_sink=$(get_default_sink) || die "No default sink found"
    default_source=$(get_default_source) || die "No microphone found"

    echo "Default speaker: $default_sink"
    echo "Default mic:     $default_source"

    # 1. Create null sink (the mixing point)
    module_id=$(pactl load-module module-null-sink \
        sink_name="$SINK_NAME" \
        "sink_properties=device.description=$SINK_NAME" \
        channel_map=stereo)
    echo "$module_id" > "$MODULE_FILE"
    echo "Created null sink: $SINK_NAME (module $module_id)"

    # 2. Create remap-source so "Omanote" appears as a selectable mic
    remap_module_id=$(pactl load-module module-remap-source \
        source_name="$SOURCE_NAME" \
        "master=${SINK_NAME}.monitor" \
        "source_properties=device.description=$SOURCE_NAME")
    echo "$remap_module_id" > "$REMAP_MODULE_FILE"
    echo "Created remap source: $SOURCE_NAME (module $remap_module_id)"

    # 3. Loopback: physical mic → virtual sink
    pw-loopback -C "$default_source" -P "$SINK_NAME" -n omanote-mic &
    local mic_pid=$!

    # 4. Loopback: system audio monitor → virtual sink
    pw-loopback -C "${default_sink}.monitor" -P "$SINK_NAME" -n omanote-sys &
    local sys_pid=$!

    echo "$mic_pid" > "$PID_FILE"
    echo "$sys_pid" >> "$PID_FILE"

    echo ""
    echo "Omanote active (PIDs: $mic_pid, $sys_pid)"
    echo "Select \"$SOURCE_NAME\" as your microphone in browser/app settings."
}

cmd_stop() {
    local stopped=false

    if [[ -f "$PID_FILE" ]]; then
        while read -r pid; do
            if kill "$pid" 2>/dev/null; then
                echo "Stopped loopback (PID $pid)"
                stopped=true
            fi
        done < "$PID_FILE"
        rm -f "$PID_FILE"
    fi

    if [[ -f "$REMAP_MODULE_FILE" ]]; then
        local remap_module_id
        remap_module_id=$(cat "$REMAP_MODULE_FILE")
        if pactl unload-module "$remap_module_id" 2>/dev/null; then
            echo "Removed remap source (module $remap_module_id)"
            stopped=true
        fi
        rm -f "$REMAP_MODULE_FILE"
    fi

    if [[ -f "$MODULE_FILE" ]]; then
        local module_id
        module_id=$(cat "$MODULE_FILE")
        if pactl unload-module "$module_id" 2>/dev/null; then
            echo "Removed null sink (module $module_id)"
            stopped=true
        fi
        rm -f "$MODULE_FILE"
    fi

    if [[ "$stopped" == false ]]; then
        echo "Not running."
    else
        echo "Omanote stopped."
    fi
}

cmd_status() {
    if is_running 2>/dev/null; then
        echo "Running"
        echo "  Loopback PIDs: $(cat "$PID_FILE" | tr '\n' ' ')"
        [[ -f "$MODULE_FILE" ]] && echo "  Null sink module: $(cat "$MODULE_FILE")"
        [[ -f "$REMAP_MODULE_FILE" ]] && echo "  Remap source module: $(cat "$REMAP_MODULE_FILE")"
        echo ""
        echo "Sources:"
        pactl list sources short | grep -E "$SINK_NAME|$SOURCE_NAME|$(get_default_source 2>/dev/null || true)" || true
    else
        # Clean up stale files
        rm -f "$PID_FILE" "$MODULE_FILE" "$REMAP_MODULE_FILE"
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

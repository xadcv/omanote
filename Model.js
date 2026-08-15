.pragma library

var pluginId = "xadcv.omanote"

function emptyStatus() {
    return {
        daemon_running: false,
        run_state: { running: false },
        rec_state: "off",
        rec_elapsed_secs: 0,
        output_dir: "",
        last_saved: "",
        error: "",
        monitor_available: false,
        preferred_source: "",
        preferred_sink: "",
        autostart: false,
        state: "inactive"
    }
}

function emptyDevices() {
    return { sources: [], sinks: [] }
}

function parseJSON(raw, fallback) {
    try {
        var parsed = JSON.parse(String(raw || ""))
        if (parsed && typeof parsed === "object") return parsed
    } catch (e) {
    }
    return fallback
}

function parseStatus(raw) {
    var parsed = parseJSON(raw, null)
    if (!parsed) return emptyStatus()
    if (!parsed.run_state || typeof parsed.run_state !== "object")
        parsed.run_state = { running: false }
    if (!parsed.rec_state) parsed.rec_state = "off"
    if (typeof parsed.rec_elapsed_secs !== "number") parsed.rec_elapsed_secs = 0
    if (!parsed.state) parsed.state = statusState(parsed)
    return parsed
}

function parseDevices(raw) {
    var parsed = parseJSON(raw, null)
    if (!parsed) return emptyDevices()
    return {
        sources: Array.isArray(parsed.sources) ? parsed.sources : [],
        sinks: Array.isArray(parsed.sinks) ? parsed.sinks : []
    }
}

function statusState(status) {
    if (!status) return "inactive"
    if (status.error) return "error"
    if (status.state) return String(status.state)
    if (!status.daemon_running) return "inactive"
    if (status.rec_state === "recording") return "recording"
    if (status.rec_state === "pending") return "pending"
    if (status.run_state && status.run_state.running) return "live"
    return "idle"
}

function formatDuration(totalSeconds) {
    var n = parseInt(totalSeconds, 10)
    if (!isFinite(n) || n < 0) n = 0
    var mins = Math.floor(n / 60)
    var secs = n % 60
    return String(mins).padStart(2, "0") + ":" + String(secs).padStart(2, "0")
}

function statusTooltip(status) {
    if (!status) return "Omanote"
    if (status.error) return String(status.error)
    var state = statusState(status)
    if (state === "inactive") return "Omanote daemon stopped"
    if (state === "recording") return "Omanote recording " + formatDuration(status.rec_elapsed_secs)
    if (state === "pending") return "Omanote recording stopped; save or discard"
    if (state === "live") return "Omanote virtual mic live"
    if (state === "error") return String(status.error || "Omanote error")
    return "Omanote ready"
}

function statusTitle(status) {
    switch (statusState(status)) {
    case "live":
        return "Virtual mic live"
    case "recording":
        return "Recording " + formatDuration(status.rec_elapsed_secs)
    case "pending":
        return "Save or discard"
    case "error":
        return "Omanote error"
    case "idle":
        return "Omanote ready"
    default:
        return "Omanote stopped"
    }
}

function statusIcon(state) {
    switch (String(state || "")) {
    case "live":
        return String.fromCodePoint(0xF034C)
    case "recording":
        return String.fromCodePoint(0xF044B)
    case "pending":
        return String.fromCodePoint(0xF0193)
    case "error":
        return String.fromCodePoint(0xF015A)
    default:
        return String.fromCodePoint(0xF034D)
    }
}

function deviceLabel(device) {
    if (!device) return ""
    if (device.description) return String(device.description)
    return String(device.name || "")
}

function deviceName(device) {
    return device && device.name ? String(device.name) : ""
}

function isSelectedDevice(device, selectedName) {
    return deviceName(device) === String(selectedName || "")
}

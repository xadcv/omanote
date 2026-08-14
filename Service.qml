import QtQuick
import Quickshell
import Quickshell.Io
import "Model.js" as Model

Item {
    id: root

    property bool installed: false
    property bool tuiHelperInstalled: false
    property bool refreshing: false
    property var status: Model.emptyStatus()
    property var sources: []
    property var sinks: []
    property string lastError: ""
    property string actionStatus: ""

    readonly property string state: Model.statusState(status)
    readonly property string icon: Model.statusIcon(state)
    readonly property string tooltip: Model.statusTooltip(status)
    readonly property bool live: !!(status && status.run_state && status.run_state.running)
    readonly property string recState: status && status.rec_state ? String(status.rec_state) : "off"
    readonly property bool busy: whichProcess.running || helperProcess.running || statusProcess.running || devicesProcess.running || actionProcess.running

    function refresh() {
        if (!installed) {
            if (!whichProcess.running) {
                refreshing = true
                whichProcess.command = ["which", "omanote"]
                whichProcess.running = true
            }
            return
        }
        refreshStatus()
    }

    function refreshAll() {
        refresh()
        if (installed) refreshDevices()
    }

    function refreshStatus() {
        if (!installed || statusProcess.running) return
        statusProcess.command = ["omanote", "status", "--json"]
        statusProcess.running = true
    }

    function refreshDevices() {
        if (!installed || devicesProcess.running) return
        devicesProcess.command = ["omanote", "devices", "--json"]
        devicesProcess.running = true
    }

    function runAction(args, label) {
        if (!installed || actionProcess.running) return
        lastError = ""
        actionStatus = label || ""
        actionProcess.command = ["omanote"].concat(args)
        actionProcess.running = true
    }

    function startMic(mic, sink) {
        var args = ["start"]
        if (mic) args.push(String(mic))
        if (sink) args.push(String(sink))
        runAction(args, "Starting virtual mic…")
    }

    function stopMic() {
        runAction(["stop"], "Stopping virtual mic…")
    }

    function toggleMic(mic, sink) {
        if (live) stopMic()
        else startMic(mic, sink)
    }

    function startRecording() {
        runAction(["record", "start"], "Starting recording…")
    }

    function stopRecording() {
        runAction(["record", "stop"], "Stopping recording…")
    }

    function saveRecording() {
        runAction(["record", "save"], "Saving recording…")
    }

    function discardRecording() {
        runAction(["record", "discard"], "Discarding recording…")
    }

    function setAutostart(enabled) {
        runAction(["autostart", enabled ? "enable" : "disable"], enabled ? "Enabling start on login…" : "Disabling start on login…")
    }

    function quitDaemon() {
        runAction(["quit"], "Stopping Omanote…")
    }

    function openTui() {
        if (tuiHelperInstalled)
            Quickshell.execDetached(["omarchy-launch-or-focus-tui", "omanote"])
        else if (installed)
            Quickshell.execDetached(["omanote"])
    }

    function elideStatus(text) {
        var value = String(text || "").replace(/\s+/g, " ").trim()
        return value.length > 140 ? value.substring(0, 137) + "…" : value
    }

    Timer {
        id: refreshTimer
        interval: 1000
        repeat: true
        running: true
        triggeredOnStart: true
        onTriggered: root.refresh()
    }

    Timer {
        id: devicesTimer
        interval: 5000
        repeat: true
        running: true
        triggeredOnStart: true
        onTriggered: if (root.installed) root.refreshDevices()
    }

    Timer {
        id: delayedRefresh
        interval: 400
        repeat: false
        onTriggered: root.refreshAll()
    }

    Timer {
        id: actionStatusTimer
        interval: 2200
        repeat: false
        onTriggered: root.actionStatus = ""
    }

    Process {
        id: whichProcess
        running: false
        command: []
        onExited: function(exitCode) {
            root.installed = exitCode === 0
            if (!helperProcess.running) {
                helperProcess.command = ["which", "omarchy-launch-or-focus-tui"]
                helperProcess.running = true
            }
            if (root.installed) {
                root.refreshStatus()
                root.refreshDevices()
            } else {
                root.refreshing = false
                root.status = Model.emptyStatus()
                root.sources = []
                root.sinks = []
                root.lastError = ""
            }
        }
    }

    Process {
        id: helperProcess
        running: false
        command: []
        onExited: function(exitCode) {
            root.tuiHelperInstalled = exitCode === 0
        }
    }

    Process {
        id: statusProcess
        running: false
        command: []
        stdout: StdioCollector {
            id: statusStdout
            waitForEnd: true
        }
        stderr: StdioCollector {
            id: statusStderr
            waitForEnd: true
        }
        onExited: function(exitCode) {
            root.refreshing = false
            var stdout = String(statusStdout.text || "")
            var stderr = String(statusStderr.text || "")
            if (exitCode === 0) {
                root.status = Model.parseStatus(stdout)
                if (root.status.error)
                    root.lastError = root.status.error
                else
                    root.lastError = ""
            } else {
                root.status = Model.emptyStatus()
                root.lastError = root.elideStatus(stderr || stdout || "Could not read Omanote status")
            }
        }
    }

    Process {
        id: devicesProcess
        running: false
        command: []
        stdout: StdioCollector {
            id: devicesStdout
            waitForEnd: true
        }
        stderr: StdioCollector {
            id: devicesStderr
            waitForEnd: true
        }
        onExited: function(exitCode) {
            var stdout = String(devicesStdout.text || "")
            if (exitCode === 0) {
                var parsed = Model.parseDevices(stdout)
                root.sources = parsed.sources
                root.sinks = parsed.sinks
            }
        }
    }

    Process {
        id: actionProcess
        running: false
        command: []
        stdout: StdioCollector {
            id: actionStdout
            waitForEnd: true
        }
        stderr: StdioCollector {
            id: actionStderr
            waitForEnd: true
        }
        onExited: function(exitCode) {
            var stdout = String(actionStdout.text || "")
            var stderr = String(actionStderr.text || "")
            if (exitCode !== 0) {
                root.lastError = root.elideStatus(stderr || stdout || "Omanote command failed")
                root.actionStatus = root.lastError
                actionStatusTimer.restart()
            } else {
                root.lastError = ""
                root.actionStatus = ""
            }
            delayedRefresh.restart()
        }
    }
}

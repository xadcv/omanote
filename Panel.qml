import QtQuick
import Quickshell
import qs.Commons
import qs.Ui
import "Model.js" as Model

Panel {
    id: root
    moduleName: "xadcv.omanote"
    ipcTarget: "xadcv.omanote"
    manageIpc: false

    property var anchorItem: null
    property var hostWidget: null
    property bool openedFromHotkey: false
    readonly property var barIdentity: hostWidget || root
    readonly property var omanote: service
    readonly property color contentForeground: bar ? bar.foreground : Color.foreground
    readonly property string contentFontFamily: bar ? bar.fontFamily : Style.font.family
    readonly property color dim: Qt.darker(contentForeground, 1.55)
    readonly property color hoverFill: bar ? Style.hoverFillFor(bar.foreground, Color.accent) : "transparent"

    property string selectedSource: ""
    property string selectedSink: ""
    property bool selectionTouched: false
    property string focusSection: "actions"
    property int selectedIndex: 0
    property bool cursorActive: false

    readonly property var actionItems: visibleActions()
    readonly property bool devicesEditable: service.installed && !service.live && service.recState !== "recording"

    function open() {
        openedFromHotkey = false
        setCenterHoverRevealSuppressed(false)
        service.refreshAll()
        root.controller.show()
    }

    function openFromHotkey() {
        openedFromHotkey = true
        service.refreshAll()
        root.controller.show()
        Qt.callLater(function() {
            if (root.opened) setCenterHoverRevealSuppressed(true)
        })
    }

    function close() {
        setCenterHoverRevealSuppressed(false)
        root.controller.hide()
    }

    function toggle() {
        if (root.opened) root.close()
        else root.openFromHotkey()
    }

    function switchPanel(direction) {
        if (root.bar && typeof root.bar.switchPanelFrom === "function")
            return root.bar.switchPanelFrom(root.barIdentity, direction)
        return false
    }

    function setCenterHoverRevealSuppressed(value) {
        if (root.bar && "centerHoverRevealSuppressed" in root.bar)
            root.bar.centerHoverRevealSuppressed = value
    }

    function refresh() {
        service.refreshAll()
    }

    function syncSelectionFromStatus() {
        if (root.selectionTouched) return
        if (service.status && service.status.preferred_source)
            root.selectedSource = service.status.preferred_source
        if (service.status && service.status.preferred_sink)
            root.selectedSink = service.status.preferred_sink
    }

    function selectSource(name) {
        if (!root.devicesEditable) return
        root.selectedSource = String(name || "")
        root.selectionTouched = true
    }

    function selectSink(name) {
        if (!root.devicesEditable) return
        root.selectedSink = String(name || "")
        root.selectionTouched = true
    }

    function visibleActions() {
        var items = []
        if (!service.installed) return items
        if (service.recState === "pending") {
            items.push({ id: "save", label: "Save recording" })
            items.push({ id: "discard", label: "Discard recording" })
        }
        if (service.live) {
            items.push({ id: "stop-mic", label: "Stop virtual mic" })
            if (service.recState === "recording")
                items.push({ id: "stop-rec", label: "Stop recording" })
            else if (service.recState !== "pending")
                items.push({ id: "start-rec", label: "Start recording" })
        } else {
            items.push({ id: "start-mic", label: "Start virtual mic" })
        }
        items.push({ id: "open-tui", label: "Open TUI" })
        if (service.status && service.status.daemon_running)
            items.push({ id: "quit", label: "Quit daemon" })
        return items
    }

    function runAction(id) {
        switch (String(id || "")) {
        case "start-mic":
            service.startMic(root.selectedSource, root.selectedSink)
            break
        case "stop-mic":
            service.stopMic()
            break
        case "start-rec":
            service.startRecording()
            break
        case "stop-rec":
            service.stopRecording()
            break
        case "save":
            service.saveRecording()
            break
        case "discard":
            service.discardRecording()
            break
        case "open-tui":
            service.openTui()
            root.close()
            break
        case "quit":
            service.quitDaemon()
            break
        }
    }

    function sectionCount(section) {
        if (section === "actions") return actionItems.length
        if (section === "sources") return service.sources.length
        if (section === "sinks") return service.sinks.length
        if (section === "footer") return 1
        return 0
    }

    function sectionVisible(section) {
        return sectionCount(section) > 0
    }

    function nextSection(section, direction) {
        var order = ["actions", "sources", "sinks", "footer"]
        var index = order.indexOf(section)
        for (var step = 0; step < order.length; step++) {
            index = (index + direction + order.length) % order.length
            if (sectionVisible(order[index])) return order[index]
        }
        return section
    }

    function moveCursor(dx, dy) {
        if (!cursorActive) {
            cursorActive = true
            return
        }
        if (dy !== 0) {
            var count = sectionCount(focusSection)
            if (count <= 0) {
                focusSection = nextSection(focusSection, dy)
                selectedIndex = 0
                return
            }
            var next = selectedIndex + dy
            if (next < 0) {
                focusSection = nextSection(focusSection, -1)
                selectedIndex = Math.max(0, sectionCount(focusSection) - 1)
            } else if (next >= count) {
                focusSection = nextSection(focusSection, 1)
                selectedIndex = 0
            } else {
                selectedIndex = next
            }
        }
    }

    function activateCursor() {
        if (focusSection === "actions" && actionItems[selectedIndex])
            runAction(actionItems[selectedIndex].id)
        else if (focusSection === "sources" && service.sources[selectedIndex])
            selectSource(Model.deviceName(service.sources[selectedIndex]))
        else if (focusSection === "sinks" && service.sinks[selectedIndex])
            selectSink(Model.deviceName(service.sinks[selectedIndex]))
        else if (focusSection === "footer")
            service.setAutostart(!(service.status && service.status.autostart))
    }

    onOpenedChanged: if (opened) {
        cursorActive = false
        focusSection = "actions"
        selectedIndex = 0
        if (panelFlick) panelFlick.contentY = 0
        service.refreshAll()
        Qt.callLater(function() { if (keyCatcher) keyCatcher.forceActiveFocus() })
    }

    Service {
        id: service
    }

    Connections {
        target: service
        function onStatusChanged() { root.syncSelectionFromStatus() }
    }

    KeyboardPanel {
        id: panel
        anchorItem: root.anchorItem
        owner: root.barIdentity
        bar: root.bar
        open: root.opened
        focusTarget: keyCatcher
        contentWidth: panel.fittedContentWidth(Style.space(320))
        contentHeight: panel.fittedContentHeight(content.implicitHeight, Style.space(560))

        PanelKeyCatcher {
            id: keyCatcher
            anchors.fill: parent
            onMoveRequested: function(dx, dy) { root.moveCursor(dx, dy) }
            onActivateRequested: root.activateCursor()
            onCloseRequested: root.close()
            onTabRequested: function(direction) { root.switchPanel(direction) }
            onTextKey: function(t) {
                if (t === " ") service.toggleMic(root.selectedSource, root.selectedSink)
                else if (t === "r" || t === "R") {
                    if (service.recState === "recording") service.stopRecording()
                    else if (service.live && service.recState !== "pending") service.startRecording()
                } else if (t === "s" || t === "S") {
                    if (service.recState === "pending") service.saveRecording()
                } else if (t === "d" || t === "D") {
                    if (service.recState === "pending") service.discardRecording()
                } else if (t === "a" || t === "A") {
                    service.setAutostart(!(service.status && service.status.autostart))
                } else if (t === "o" || t === "O") {
                    service.openTui()
                    root.close()
                }
            }

            Flickable {
                id: panelFlick
                anchors.fill: parent
                contentWidth: width
                contentHeight: content.implicitHeight
                clip: true
                boundsBehavior: Flickable.StopAtBounds
                flickableDirection: Flickable.VerticalFlick
                interactive: contentHeight > height

                Column {
                    id: content
                    width: panelFlick.width
                    spacing: Style.space(12)

                    Column {
                        width: parent.width
                        spacing: Style.space(4)

                        Text {
                            width: parent.width
                            text: service.installed ? Model.statusTitle(service.status) : "Omanote not installed"
                            color: root.contentForeground
                            font.family: root.contentFontFamily
                            font.pixelSize: Style.font.subtitle
                            font.bold: true
                            wrapMode: Text.WordWrap
                        }

                        Text {
                            width: parent.width
                            visible: service.installed
                            text: service.actionStatus || service.lastError || Model.statusTooltip(service.status)
                            color: service.lastError ? (root.bar && root.bar.urgent ? root.bar.urgent : root.contentForeground) : root.dim
                            font.family: root.contentFontFamily
                            font.pixelSize: Style.font.body
                            wrapMode: Text.WordWrap
                        }

                        Text {
                            width: parent.width
                            visible: !service.installed
                            text: "Install the omanote binary, then restart the shell. The bar widget talks to that CLI."
                            color: root.dim
                            font.family: root.contentFontFamily
                            font.pixelSize: Style.font.body
                            wrapMode: Text.WordWrap
                        }
                    }

                    Column {
                        width: parent.width
                        spacing: Style.space(2)
                        visible: service.installed && root.actionItems.length > 0

                        Repeater {
                            model: root.actionItems

                            Rectangle {
                                required property var modelData
                                required property int index
                                width: parent.width
                                implicitHeight: actionLabel.implicitHeight + Style.space(12)
                                radius: Style.cornerRadius
                                color: (actionArea.containsMouse || (root.cursorActive && root.focusSection === "actions" && root.selectedIndex === index)) ? root.hoverFill : "transparent"

                                Text {
                                    id: actionLabel
                                    anchors.left: parent.left
                                    anchors.right: parent.right
                                    anchors.verticalCenter: parent.verticalCenter
                                    anchors.leftMargin: Style.space(8)
                                    anchors.rightMargin: Style.space(8)
                                    text: modelData.label
                                    color: root.contentForeground
                                    font.family: root.contentFontFamily
                                    font.pixelSize: Style.font.body
                                    elide: Text.ElideRight
                                }

                                MouseArea {
                                    id: actionArea
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    cursorShape: Qt.PointingHandCursor
                                    onContainsMouseChanged: if (containsMouse) {
                                        root.cursorActive = true
                                        root.focusSection = "actions"
                                        root.selectedIndex = index
                                    }
                                    onClicked: root.runAction(modelData.id)
                                }
                            }
                        }
                    }

                    Column {
                        width: parent.width
                        spacing: Style.space(2)
                        visible: service.installed && service.sources.length > 0

                        Text {
                            text: "MICROPHONE"
                            color: root.dim
                            font.family: root.contentFontFamily
                            font.pixelSize: Style.font.bodySmall
                            font.letterSpacing: 1
                        }

                        Repeater {
                            model: service.sources

                            Rectangle {
                                required property var modelData
                                required property int index
                                width: parent.width
                                implicitHeight: sourceLabel.implicitHeight + Style.space(10)
                                radius: Style.cornerRadius
                                opacity: root.devicesEditable ? 1 : 0.55
                                color: (sourceArea.containsMouse || (root.cursorActive && root.focusSection === "sources" && root.selectedIndex === index)) ? root.hoverFill : "transparent"

                                Text {
                                    id: sourceLabel
                                    anchors.left: parent.left
                                    anchors.right: parent.right
                                    anchors.verticalCenter: parent.verticalCenter
                                    anchors.leftMargin: Style.space(8)
                                    anchors.rightMargin: Style.space(8)
                                    text: Model.deviceLabel(modelData)
                                    color: root.contentForeground
                                    font.family: root.contentFontFamily
                                    font.pixelSize: Style.font.body
                                    font.bold: Model.isSelectedDevice(modelData, root.selectedSource)
                                    elide: Text.ElideRight
                                }

                                MouseArea {
                                    id: sourceArea
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    cursorShape: root.devicesEditable ? Qt.PointingHandCursor : Qt.ArrowCursor
                                    onContainsMouseChanged: if (containsMouse) {
                                        root.cursorActive = true
                                        root.focusSection = "sources"
                                        root.selectedIndex = index
                                    }
                                    onClicked: root.selectSource(Model.deviceName(modelData))
                                }
                            }
                        }
                    }

                    Column {
                        width: parent.width
                        spacing: Style.space(2)
                        visible: service.installed && service.sinks.length > 0

                        Text {
                            text: "SYSTEM OUTPUT"
                            color: root.dim
                            font.family: root.contentFontFamily
                            font.pixelSize: Style.font.bodySmall
                            font.letterSpacing: 1
                        }

                        Repeater {
                            model: service.sinks

                            Rectangle {
                                required property var modelData
                                required property int index
                                width: parent.width
                                implicitHeight: sinkLabel.implicitHeight + Style.space(10)
                                radius: Style.cornerRadius
                                opacity: root.devicesEditable ? 1 : 0.55
                                color: (sinkArea.containsMouse || (root.cursorActive && root.focusSection === "sinks" && root.selectedIndex === index)) ? root.hoverFill : "transparent"

                                Text {
                                    id: sinkLabel
                                    anchors.left: parent.left
                                    anchors.right: parent.right
                                    anchors.verticalCenter: parent.verticalCenter
                                    anchors.leftMargin: Style.space(8)
                                    anchors.rightMargin: Style.space(8)
                                    text: Model.deviceLabel(modelData)
                                    color: root.contentForeground
                                    font.family: root.contentFontFamily
                                    font.pixelSize: Style.font.body
                                    font.bold: Model.isSelectedDevice(modelData, root.selectedSink)
                                    elide: Text.ElideRight
                                }

                                MouseArea {
                                    id: sinkArea
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    cursorShape: root.devicesEditable ? Qt.PointingHandCursor : Qt.ArrowCursor
                                    onContainsMouseChanged: if (containsMouse) {
                                        root.cursorActive = true
                                        root.focusSection = "sinks"
                                        root.selectedIndex = index
                                    }
                                    onClicked: root.selectSink(Model.deviceName(modelData))
                                }
                            }
                        }
                    }

                    Rectangle {
                        width: parent.width
                        implicitHeight: autostartLabel.implicitHeight + Style.space(12)
                        radius: Style.cornerRadius
                        visible: service.installed
                        color: (autostartArea.containsMouse || (root.cursorActive && root.focusSection === "footer")) ? root.hoverFill : "transparent"

                        Text {
                            id: autostartLabel
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.verticalCenter: parent.verticalCenter
                            anchors.leftMargin: Style.space(8)
                            anchors.rightMargin: Style.space(8)
                            text: (service.status && service.status.autostart) ? "Start on login: on" : "Start on login: off"
                            color: root.contentForeground
                            font.family: root.contentFontFamily
                            font.pixelSize: Style.font.body
                        }

                        MouseArea {
                            id: autostartArea
                            anchors.fill: parent
                            hoverEnabled: true
                            cursorShape: Qt.PointingHandCursor
                            onContainsMouseChanged: if (containsMouse) {
                                root.cursorActive = true
                                root.focusSection = "footer"
                                root.selectedIndex = 0
                            }
                            onClicked: service.setAutostart(!(service.status && service.status.autostart))
                        }
                    }
                }
            }
        }
    }
}

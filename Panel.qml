import QtQuick
import QtQuick.Controls
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

    readonly property color foreground: bar ? bar.foreground : Color.foreground
    readonly property color urgent: bar ? bar.urgent : Color.urgent
    readonly property color dim: Qt.darker(foreground, 1.4)
    readonly property string fontFamily: bar ? bar.fontFamily : Style.font.family
    readonly property color hoverFill: bar ? Style.hoverFillFor(bar.foreground, Color.accent) : "transparent"
    readonly property color selectedFill: bar ? Style.selectedFillFor(bar.foreground, Color.accent) : "transparent"

    property string selectedSource: ""
    property string selectedSink: ""
    property bool selectionTouched: false
    property string focusSection: "header"
    property int selectedIndex: -1
    property bool cursorActive: false

    readonly property var recordingItems: visibleRecordingItems()
    readonly property var extraItems: visibleExtraItems()
    readonly property var moreItems: extraItems.length > 1 ? extraItems.slice(1) : []
    readonly property bool devicesEditable: service.installed && !service.live && service.recState !== "recording"
    readonly property bool headerHasCursor: cursorActive && focusSection === "header" && service.installed
    readonly property bool heroActive: service.live || service.recState === "recording" || service.recState === "pending"
    readonly property string toggleHint: Model.toggleHint(service.live)

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

    function visibleRecordingItems() {
        var items = []
        if (!service.installed) return items
        if (service.recState === "pending") {
            items.push({ id: "save", label: "Save recording", icon: Model.actionIcon("save") })
            items.push({ id: "discard", label: "Discard recording", icon: Model.actionIcon("discard") })
            return items
        }
        if (!service.live) return items
        if (service.recState === "recording")
            items.push({ id: "stop-rec", label: "Stop recording", icon: Model.actionIcon("stop-rec") })
        else
            items.push({ id: "start-rec", label: "Start recording", icon: Model.actionIcon("start-rec") })
        return items
    }

    function visibleExtraItems() {
        var items = []
        if (!service.installed) return items
        items.push({ id: "autostart", label: "Start on login", icon: "" })
        items.push({ id: "open-tui", label: "Open TUI", icon: Model.actionIcon("open-tui") })
        if (service.status && service.status.daemon_running)
            items.push({ id: "quit", label: "Quit daemon", icon: Model.actionIcon("quit") })
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
        case "autostart":
            service.setAutostart(!(service.status && service.status.autostart))
            break
        }
    }

    function sectionCount(section) {
        if (section === "recording") return recordingItems.length
        if (section === "sources") return service.sources.length
        if (section === "sinks") return service.sinks.length
        if (section === "extras") return extraItems.length
        return 0
    }

    function visibleSectionOrder() {
        var order = []
        if (recordingItems.length > 0) order.push("recording")
        if (service.sources.length > 0) order.push("sources")
        if (service.sinks.length > 0) order.push("sinks")
        if (extraItems.length > 0) order.push("extras")
        return order
    }

    function setHeaderCursor() {
        cursorActive = true
        focusSection = "header"
        selectedIndex = -1
    }

    function setRowCursor(section, index) {
        cursorActive = true
        focusSection = section
        selectedIndex = index
    }

    function moveCursor(dx, dy) {
        if (!cursorActive) {
            cursorActive = true
            return
        }
        if (dy === 0) return

        if (focusSection === "header") {
            if (dy > 0) {
                var first = visibleSectionOrder()
                if (first.length > 0) {
                    focusSection = first[0]
                    selectedIndex = 0
                }
            }
            return
        }

        var count = sectionCount(focusSection)
        var next = selectedIndex + dy
        if (next >= 0 && next < count) {
            selectedIndex = next
            return
        }

        var sections = visibleSectionOrder()
        var sIdx = sections.indexOf(focusSection)
        if (next < 0) {
            if (sIdx > 0) {
                focusSection = sections[sIdx - 1]
                selectedIndex = Math.max(0, sectionCount(focusSection) - 1)
            } else {
                focusSection = "header"
                selectedIndex = -1
            }
        } else if (sIdx >= 0 && sIdx < sections.length - 1) {
            focusSection = sections[sIdx + 1]
            selectedIndex = 0
        }
    }

    function activateCursor() {
        if (focusSection === "header") {
            if (service.installed) service.toggleMic(root.selectedSource, root.selectedSink)
            return
        }
        if (focusSection === "recording" && recordingItems[selectedIndex])
            runAction(recordingItems[selectedIndex].id)
        else if (focusSection === "sources" && service.sources[selectedIndex])
            selectSource(Model.deviceName(service.sources[selectedIndex]))
        else if (focusSection === "sinks" && service.sinks[selectedIndex])
            selectSink(Model.deviceName(service.sinks[selectedIndex]))
        else if (focusSection === "extras" && extraItems[selectedIndex])
            runAction(extraItems[selectedIndex].id)
    }

    function clampCursor() {
        if (focusSection === "header") return
        var sections = visibleSectionOrder()
        if (!sections.length) {
            focusSection = "header"
            selectedIndex = -1
            return
        }
        if (sections.indexOf(focusSection) < 0) {
            focusSection = sections[0]
            selectedIndex = 0
            return
        }
        var count = sectionCount(focusSection)
        if (selectedIndex > count - 1) selectedIndex = Math.max(0, count - 1)
        if (selectedIndex < 0) selectedIndex = 0
    }

    function ensureCursorVisible(item) {
        if (!item || !panelFlick) return
        var margin = 6
        var maxY = Math.max(0, panelFlick.contentHeight - panelFlick.height)
        if (maxY <= Style.space(24) || focusSection === "header") {
            panelFlick.contentY = 0
            return
        }
        var pt = item.mapToItem(panelFlick.contentItem, 0, 0)
        var top = pt.y
        var bottom = top + (item.height || 0)
        var viewTop = panelFlick.contentY
        var viewBottom = viewTop + panelFlick.height
        if (top < viewTop + margin)
            panelFlick.contentY = Math.max(0, Math.min(maxY, top - margin))
        else if (bottom > viewBottom - margin)
            panelFlick.contentY = Math.max(0, Math.min(maxY, bottom + margin - panelFlick.height))
    }

    onOpenedChanged: if (opened) {
        cursorActive = false
        focusSection = "header"
        selectedIndex = -1
        if (panelFlick) panelFlick.contentY = 0
        service.refreshAll()
        Qt.callLater(function() { if (keyCatcher) keyCatcher.forceActiveFocus() })
    }

    onRecordingItemsChanged: clampCursor()
    onExtraItemsChanged: clampCursor()

    Service {
        id: service
    }

    Connections {
        target: service
        function onStatusChanged() { root.syncSelectionFromStatus() }
        function onSourcesChanged() { root.clampCursor() }
        function onSinksChanged() { root.clampCursor() }
    }

    KeyboardPanel {
        id: panel
        anchorItem: root.anchorItem
        owner: root.barIdentity
        bar: root.bar
        open: root.opened
        focusTarget: keyCatcher
        contentWidth: panel.fittedContentWidth(Style.space(380))
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
                ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

                Column {
                    id: content
                    width: panelFlick.width
                    spacing: Style.space(12)

                    Item {
                        id: header
                        width: parent.width
                        implicitHeight: hero.implicitHeight
                        readonly property bool ringVisible: root.headerHasCursor
                        function focusHero() { root.setHeaderCursor() }

                        PanelHero {
                            id: hero
                            width: parent.width
                            title: "Omanote"
                            meta: Model.heroCaption(service.status, service.installed)
                            detail: Model.heroDetail(service.status)
                            foreground: root.foreground
                            fontFamily: root.fontFamily
                            iconOpacity: root.heroActive || !!service.lastError ? 1.0 : 0.5
                            iconComponent: Component {
                                Text {
                                    text: service.installed ? Model.statusIcon(service.state) : "󰍭"
                                    color: root.foreground
                                    font.family: root.fontFamily
                                    font.pixelSize: Style.font.display
                                }
                            }
                            trailingControl: Component {
                                ToggleSwitch {
                                    id: powerSwitch
                                    visible: service.installed
                                    checked: service.live
                                    busy: service.busy
                                    hasCursor: header.ringVisible
                                    foreground: hero.foreground
                                    onHovered: function(on) { if (on) header.focusHero() }
                                    onToggled: service.toggleMic(root.selectedSource, root.selectedSink)

                                    PanelToolTip {
                                        visible: powerSwitch.containsMouse
                                        text: root.toggleHint
                                        fontFamily: hero.fontFamily
                                    }
                                }
                            }
                        }
                    }

                    Text {
                        visible: service.actionStatus !== "" || service.lastError !== ""
                        width: parent.width
                        text: service.actionStatus !== "" ? service.actionStatus : service.lastError
                        color: service.lastError !== "" && service.actionStatus === "" ? root.urgent : root.dim
                        font.family: root.fontFamily
                        font.pixelSize: Style.font.bodySmall
                        wrapMode: Text.WordWrap
                    }

                    Text {
                        visible: !service.installed
                        width: parent.width
                        text: "Install the omanote binary, then restart the shell."
                        color: root.dim
                        font.family: root.fontFamily
                        font.pixelSize: Style.font.body
                        wrapMode: Text.WordWrap
                    }

                    PanelSeparator {
                        visible: service.installed && root.recordingItems.length > 0
                        foreground: root.foreground
                    }

                    Column {
                        visible: service.installed && root.recordingItems.length > 0
                        width: parent.width
                        spacing: Style.space(6)

                        PanelSectionHeader {
                            text: "RECORDING"
                            foreground: root.foreground
                            fontFamily: root.fontFamily
                        }

                        Repeater {
                            model: root.recordingItems
                            ActionRow {
                                required property var modelData
                                required property int index
                                width: content.width
                                item: modelData
                                rowIndex: index
                                sectionName: "recording"
                            }
                        }
                    }

                    PanelSeparator {
                        visible: service.installed && service.sources.length > 0
                        foreground: root.foreground
                    }

                    Column {
                        visible: service.installed && service.sources.length > 0
                        width: parent.width
                        spacing: Style.space(6)
                        opacity: root.devicesEditable ? 1 : 0.55

                        Item {
                            width: parent.width
                            implicitHeight: Math.max(inputHeader.implicitHeight, inputLock.implicitHeight)

                            PanelSectionHeader {
                                id: inputHeader
                                text: "INPUT"
                                foreground: root.foreground
                                fontFamily: root.fontFamily
                                anchors.left: parent.left
                                anchors.verticalCenter: parent.verticalCenter
                            }

                            Text {
                                id: inputLock
                                visible: !root.devicesEditable
                                text: "LOCKED"
                                color: root.dim
                                font.family: root.fontFamily
                                font.pixelSize: Style.font.caption
                                font.bold: true
                                font.letterSpacing: 1.2
                                anchors.right: parent.right
                                anchors.rightMargin: Style.space(6)
                                anchors.verticalCenter: parent.verticalCenter
                            }
                        }

                        Repeater {
                            model: service.sources
                            DeviceRow {
                                required property var modelData
                                required property int index
                                width: content.width
                                device: modelData
                                rowIndex: index
                                sectionName: "sources"
                                glyph: Model.sourceGlyph(modelData)
                                selected: Model.isSelectedDevice(modelData, root.selectedSource)
                                onChosen: root.selectSource(Model.deviceName(device))
                            }
                        }
                    }

                    PanelSeparator {
                        visible: service.installed && service.sinks.length > 0
                        foreground: root.foreground
                    }

                    Column {
                        visible: service.installed && service.sinks.length > 0
                        width: parent.width
                        spacing: Style.space(6)
                        opacity: root.devicesEditable ? 1 : 0.55

                        Item {
                            width: parent.width
                            implicitHeight: Math.max(outputHeader.implicitHeight, outputLock.implicitHeight)

                            PanelSectionHeader {
                                id: outputHeader
                                text: "OUTPUT"
                                foreground: root.foreground
                                fontFamily: root.fontFamily
                                anchors.left: parent.left
                                anchors.verticalCenter: parent.verticalCenter
                            }

                            Text {
                                id: outputLock
                                visible: !root.devicesEditable
                                text: "LOCKED"
                                color: root.dim
                                font.family: root.fontFamily
                                font.pixelSize: Style.font.caption
                                font.bold: true
                                font.letterSpacing: 1.2
                                anchors.right: parent.right
                                anchors.rightMargin: Style.space(6)
                                anchors.verticalCenter: parent.verticalCenter
                            }
                        }

                        Repeater {
                            model: service.sinks
                            DeviceRow {
                                required property var modelData
                                required property int index
                                width: content.width
                                device: modelData
                                rowIndex: index
                                sectionName: "sinks"
                                glyph: Model.sinkGlyph(modelData)
                                selected: Model.isSelectedDevice(modelData, root.selectedSink)
                                onChosen: root.selectSink(Model.deviceName(device))
                            }
                        }
                    }

                    PanelSeparator {
                        visible: service.installed && root.extraItems.length > 0
                        foreground: root.foreground
                    }

                    Column {
                        visible: service.installed && root.extraItems.length > 0
                        width: parent.width
                        spacing: Style.space(6)

                        AutostartRow {
                            width: content.width
                            item: root.extraItems.length > 0 ? root.extraItems[0] : ({})
                            rowIndex: 0
                            sectionName: "extras"
                        }

                        Repeater {
                            model: root.moreItems
                            ActionRow {
                                required property var modelData
                                required property int index
                                width: content.width
                                item: modelData
                                rowIndex: index + 1
                                sectionName: "extras"
                            }
                        }
                    }
                }
            }
        }
    }

    component ActionRow: CursorSurface {
        id: actionRow
        property var item: ({})
        property int rowIndex: 0
        property string sectionName: "recording"

        hasCursor: root.cursorActive && root.focusSection === sectionName && root.selectedIndex === rowIndex
        onHasCursorChanged: if (hasCursor) root.ensureCursorVisible(actionRow)
        foreground: root.foreground
        fill: root.hoverFill
        implicitHeight: actionInner.implicitHeight + Style.spacing.xl

        Row {
            id: actionInner
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            anchors.leftMargin: Style.space(6)
            anchors.rightMargin: Style.space(6)
            spacing: Style.space(8)

            Text {
                visible: actionRow.item && actionRow.item.icon
                text: actionRow.item && actionRow.item.icon ? actionRow.item.icon : ""
                color: root.foreground
                font.family: root.fontFamily
                font.pixelSize: Style.font.title
                width: Style.space(22)
                horizontalAlignment: Text.AlignHCenter
                anchors.verticalCenter: parent.verticalCenter
            }

            Text {
                text: actionRow.item && actionRow.item.label ? actionRow.item.label : ""
                color: root.foreground
                font.family: root.fontFamily
                font.pixelSize: Style.font.body
                elide: Text.ElideRight
                width: parent.width - (actionRow.item && actionRow.item.icon ? Style.space(30) : 0)
                anchors.verticalCenter: parent.verticalCenter
            }
        }

        MouseArea {
            anchors.fill: parent
            hoverEnabled: true
            cursorShape: Qt.PointingHandCursor
            onContainsMouseChanged: if (containsMouse) root.setRowCursor(actionRow.sectionName, actionRow.rowIndex)
            onClicked: root.runAction(actionRow.item ? actionRow.item.id : "")
        }
    }

    component AutostartRow: CursorSurface {
        id: autostartRow
        property var item: ({})
        property int rowIndex: 0
        property string sectionName: "extras"
        readonly property bool enabled: !!(service.status && service.status.autostart)

        hasCursor: root.cursorActive && root.focusSection === sectionName && root.selectedIndex === rowIndex
        onHasCursorChanged: if (hasCursor) root.ensureCursorVisible(autostartRow)
        foreground: root.foreground
        fill: root.hoverFill
        implicitHeight: Math.max(autostartLabel.implicitHeight, autostartSwitch.implicitHeight) + Style.spacing.xl

        Text {
            id: autostartLabel
            anchors.left: parent.left
            anchors.right: autostartSwitch.left
            anchors.verticalCenter: parent.verticalCenter
            anchors.leftMargin: Style.space(6)
            anchors.rightMargin: Style.space(12)
            text: "Start on login"
            color: root.foreground
            font.family: root.fontFamily
            font.pixelSize: Style.font.body
            elide: Text.ElideRight
        }

        ToggleSwitch {
            id: autostartSwitch
            checked: autostartRow.enabled
            interactive: false
            foreground: root.foreground
            anchors.right: parent.right
            anchors.rightMargin: Style.space(6)
            anchors.verticalCenter: parent.verticalCenter
        }

        MouseArea {
            anchors.fill: parent
            hoverEnabled: true
            cursorShape: Qt.PointingHandCursor
            onContainsMouseChanged: if (containsMouse) root.setRowCursor(autostartRow.sectionName, autostartRow.rowIndex)
            onClicked: root.runAction("autostart")
        }
    }

    component DeviceRow: CursorSurface {
        id: deviceRow
        property var device: null
        property int rowIndex: 0
        property string sectionName: "sources"
        property string glyph: ""
        property bool selected: false
        signal chosen()

        hasCursor: root.cursorActive && root.focusSection === sectionName && root.selectedIndex === rowIndex
        onHasCursorChanged: if (hasCursor) root.ensureCursorVisible(deviceRow)
        current: selected
        foreground: root.foreground
        fill: root.hoverFill
        currentFill: root.selectedFill
        implicitHeight: deviceInner.implicitHeight + Style.spacing.xl

        Row {
            id: deviceInner
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            anchors.leftMargin: Style.space(6)
            anchors.rightMargin: Style.space(6)
            spacing: Style.space(8)

            Text {
                text: deviceRow.glyph
                color: root.foreground
                font.family: root.fontFamily
                font.pixelSize: Style.font.title
                width: Style.space(22)
                horizontalAlignment: Text.AlignHCenter
                anchors.verticalCenter: parent.verticalCenter
            }

            Text {
                text: Model.deviceLabel(deviceRow.device)
                color: root.foreground
                font.family: root.fontFamily
                font.pixelSize: Style.font.body
                font.bold: deviceRow.selected
                elide: Text.ElideRight
                width: parent.width - Style.space(22) - Style.space(8)
                anchors.verticalCenter: parent.verticalCenter
            }
        }

        MouseArea {
            anchors.fill: parent
            hoverEnabled: true
            cursorShape: root.devicesEditable ? Qt.PointingHandCursor : Qt.ArrowCursor
            onContainsMouseChanged: if (containsMouse) root.setRowCursor(deviceRow.sectionName, deviceRow.rowIndex)
            onClicked: deviceRow.chosen()
        }
    }

}

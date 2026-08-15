import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui
import "Model.js" as Model

BarWidget {
    id: root
    moduleName: "xadcv.omanote"

    readonly property var omanote: panelLoader.item ? panelLoader.item.omanote : null
    readonly property string iconText: omanote ? omanote.icon : Model.statusIcon("inactive")
    readonly property string tooltip: omanote ? omanote.tooltip : "Omanote"

    function injectPanel() {
        var target = panelLoader.item
        if (!target) return
        if ("bar" in target) target.bar = root.bar
        if ("settings" in target) target.settings = root.settings
        if ("anchorItem" in target) target.anchorItem = button
        if ("hostWidget" in target) target.hostWidget = root
    }

    function refresh() {
        if (panelLoader.item && panelLoader.item.refresh) panelLoader.item.refresh()
    }

    function togglePanel() {
        if (panelLoader.item && panelLoader.item.toggle) panelLoader.item.toggle()
    }

    function toggle() {
        root.togglePanel()
    }

    readonly property bool opened: panelLoader.item ? panelLoader.item.opened === true : false

    function open() {
        if (panelLoader.item && panelLoader.item.openFromHotkey)
            panelLoader.item.openFromHotkey()
        else if (panelLoader.item && panelLoader.item.open)
            panelLoader.item.open()
    }

    function close() {
        if (panelLoader.item && panelLoader.item.close) panelLoader.item.close()
    }

    readonly property bool popoutSwitchClosing: panelLoader.item ? panelLoader.item.popoutSwitchClosing === true : false

    function closeForPopoutSwitch() {
        if (panelLoader.item) panelLoader.item.closeForPopoutSwitch()
    }

    implicitWidth: button.implicitWidth
    implicitHeight: button.implicitHeight

    onBarChanged: injectPanel()
    onSettingsChanged: injectPanel()

    Loader {
        id: panelLoader
        active: true
        source: Qt.resolvedUrl("Panel.qml")
        visible: false
        onLoaded: {
            root.injectPanel()
            Qt.callLater(root.injectPanel)
        }
    }

    IpcHandler {
        target: "xadcv.omanote"

        function refresh(): void { root.refresh() }
        function open(): void { root.open() }
        function close(): void { root.close() }
        function show(): void { root.open() }
        function hide(): void { root.close() }
        function toggle(): void { root.togglePanel() }
    }

    BarIconButton {
        id: button
        anchors.fill: parent
        bar: root.bar
        text: root.iconText
        slotSize: Style.bar.statusSlot
        tooltipText: root.tooltip

        onPressed: function(buttonCode) {
            if (buttonCode === Qt.RightButton) {
                if (root.omanote) root.omanote.toggleMic(panelLoader.item ? panelLoader.item.selectedSource : "", panelLoader.item ? panelLoader.item.selectedSink : "")
            } else if (buttonCode === Qt.MiddleButton) {
                root.refresh()
            } else {
                root.togglePanel()
            }
        }
    }
}

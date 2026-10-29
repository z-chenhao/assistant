import AppKit

final class Fixture: NSObject, NSApplicationDelegate, NSTextFieldDelegate {
    var window: NSWindow!
    let input = NSTextField(string: "initial")
    let status = NSTextField(labelWithString: "committed=none")
    let count = NSTextField(labelWithString: "count=0")
    let selection = NSTextField(labelWithString: "selection=none")
    var clicks = 0
    func applicationDidFinishLaunching(_ notification: Notification) {
        window = NSWindow(contentRect: NSRect(x: 200, y: 200, width: 500, height: 260), styleMask: [.titled, .closable], backing: .buffered, defer: false)
        window.title = "Collie CUA Fixture"
        window.isReleasedWhenClosed = false
        let view = window.contentView!
        input.frame = NSRect(x: 30, y: 190, width: 420, height: 28)
        input.setAccessibilityLabel("Fixture input")
        input.identifier = NSUserInterfaceItemIdentifier("fixtureInput")
        input.delegate = self
        status.frame = NSRect(x: 30, y: 145, width: 420, height: 28)
        status.setAccessibilityLabel("Commit status")
        let button = NSButton(title: "Increment", target: self, action: #selector(increment))
        button.frame = NSRect(x: 30, y: 90, width: 150, height: 32)
        count.frame = NSRect(x: 220, y: 90, width: 200, height: 32)
        let popup = NSPopUpButton(frame: NSRect(x: 30, y: 35, width: 200, height: 28))
        popup.addItems(withTitles: ["First", "Second"])
        popup.setAccessibilityLabel("Fixture menu")
        selection.frame = NSRect(x: 250, y: 35, width: 220, height: 28)
        NotificationCenter.default.addObserver(forName: NSTextView.didChangeSelectionNotification, object: nil, queue: .main) { [weak self] event in
            if let editor = event.object as? NSTextView {
                let range = editor.selectedRange()
                self?.selection.stringValue = "selection=\(range.location):\(range.length)"
            }
        }
        for child in [input, status, button, count, popup, selection] { view.addSubview(child) }
        NSApp.setActivationPolicy(.regular)
        window.makeKeyAndOrderFront(nil)
        window.makeFirstResponder(input)
    }
    func controlTextDidEndEditing(_ notification: Notification) { status.stringValue = "committed=" + input.stringValue }
    @objc func increment() { clicks += 1; count.stringValue = "count=\(clicks)" }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
}
let app = NSApplication.shared
let delegate = Fixture()
app.delegate = delegate
app.run()

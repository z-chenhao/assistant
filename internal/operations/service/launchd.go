package service

import (
	"bytes"
	"encoding/xml"
	"os"
)

func xmlString(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return "<string>" + b.String() + "</string>"
}

func (m *Manager) plist(inspector bool) []byte {
	label, operation := m.label(), "run"
	if inspector {
		label += ".inspect"
		operation = "inspect"
	}
	var b bytes.Buffer
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\"><dict>\n<key>Label</key>")
	b.WriteString(xmlString(label))
	b.WriteString("<key>ProgramArguments</key><array>")
	for _, arg := range []string{m.executable, "service", operation, m.root} {
		b.WriteString(xmlString(arg))
	}
	b.WriteString("</array><key>EnvironmentVariables</key><dict><key>PATH</key>")
	b.WriteString(xmlString(os.Getenv("PATH")))
	b.WriteString("</dict><key>RunAtLoad</key><true/><key>ProcessType</key><string>Background</string><key>ExitTimeOut</key><integer>30</integer>")
	if inspector {
		b.WriteString("<key>StartInterval</key><integer>60</integer>")
	}
	// Stdout/stderr are intentionally discarded. Existing application telemetry
	// rotates separately; launchd files never become an unbounded prompt log.
	b.WriteString("<key>StandardOutPath</key><string>/dev/null</string><key>StandardErrorPath</key><string>/dev/null</string></dict></plist>\n")
	return b.Bytes()
}

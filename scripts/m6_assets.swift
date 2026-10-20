// Synthetic documents and an optional real Vision OCR observation. Generation
// never draws the masked year. This is fixture tooling, not a product decoder.
import AppKit
import Vision
import Foundation

let args = CommandLine.arguments
guard args.count == 3 else { fatalError("make DIRECTORY | ocr IMAGE") }
let url = URL(fileURLWithPath: args[2])
if args[1] == "make" {
    let documents = [
        "visa_front.png": ["SYNTHETIC VISA", "VALID FROM 2026-09-20", "VALID UNTIL 2026-10-20", "ENTRIES 01", "DURATION DAYS 15"],
        "flight_order.png": ["SYNTHETIC FLIGHT ORDER", "OUTBOUND 2026-10-03", "RETURN      -10-11", "YEAR MASKED / UNKNOWN"],
        "architecture.png": ["CONFIRMED ARCHITECTURE", "Main -> Task -> Root -> Child", "Root / Child -> Tooling", "Child -> direct Parent only"],
    ]
    for (name, lines) in documents {
        let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: 900, pixelsHigh: 360,
            bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
            colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: bitmap)
        NSColor.white.setFill(); NSRect(x: 0, y: 0, width: 900, height: 360).fill()
        for (index, line) in lines.enumerated() {
            (line as NSString).draw(at: NSPoint(x: 35, y: 300 - index * 58), withAttributes: [
                .font: NSFont.monospacedSystemFont(ofSize: 30, weight: .medium), .foregroundColor: NSColor.black])
        }
        if name == "flight_order.png" {
            NSColor.black.setFill(); NSRect(x: 160, y: 178, width: 90, height: 40).fill()
        }
        NSGraphicsContext.restoreGraphicsState()
        try bitmap.representation(using: .png, properties: [:])!.write(to: url.appendingPathComponent(name), options: .withoutOverwriting)
    }
} else if args[1] == "ocr" {
    let request = VNRecognizeTextRequest()
    request.recognitionLevel = .accurate
    request.usesLanguageCorrection = false
    try VNImageRequestHandler(url: url).perform([request])
    let observations: [[String: Any]] = (request.results ?? []).compactMap { result in
        guard let text = result.topCandidates(1).first else { return nil }
        let box = result.boundingBox
        return ["text": text.string, "confidence": text.confidence,
                "region": [box.minX, 1 - box.maxY, box.width, box.height],
                "coordinates": "normalized top-left"]
    }
    print(String(data: try JSONSerialization.data(withJSONObject: observations, options: [.sortedKeys]), encoding: .utf8)!)
} else { fatalError("unknown operation") }

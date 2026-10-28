// Opt-in synthetic L2 media generator and export decoder. Run through lume_live.py.
import AVFoundation
import CoreVideo
import Foundation
import CoreGraphics
import ImageIO
import UniformTypeIdentifiers

@main
struct MediaFixture {
    static func makeClip(_ url: URL, blue: Bool) async throws {
        let writer = try AVAssetWriter(outputURL: url, fileType: .mov)
        let input = AVAssetWriterInput(mediaType: .video, outputSettings: [
            AVVideoCodecKey: AVVideoCodecType.h264,
            AVVideoWidthKey: 320, AVVideoHeightKey: 180,
        ])
        input.expectsMediaDataInRealTime = false
        let adapter = AVAssetWriterInputPixelBufferAdaptor(assetWriterInput: input,
            sourcePixelBufferAttributes: [
                kCVPixelBufferPixelFormatTypeKey as String: kCVPixelFormatType_32BGRA,
                kCVPixelBufferWidthKey as String: 320,
                kCVPixelBufferHeightKey as String: 180,
                kCVPixelBufferCGImageCompatibilityKey as String: true,
                kCVPixelBufferCGBitmapContextCompatibilityKey as String: true,
            ])
        guard writer.canAdd(input) else { throw NSError(domain: "fixture.input", code: 1) }
        writer.add(input)
        guard writer.startWriting() else { throw writer.error! }
        writer.startSession(atSourceTime: .zero)
        for frame in 0..<72 {
            while !input.isReadyForMoreMediaData {
                guard writer.status == .writing else { throw writer.error! }
                try await Task.sleep(for: .milliseconds(1))
            }
            var buffer: CVPixelBuffer?
            guard let pool = adapter.pixelBufferPool,
                  CVPixelBufferPoolCreatePixelBuffer(nil, pool, &buffer) == kCVReturnSuccess,
                  let buffer else { throw NSError(domain: "fixture.buffer", code: 2) }
            CVPixelBufferLockBaseAddress(buffer, [])
            let pixels = CVPixelBufferGetBaseAddress(buffer)!.assumingMemoryBound(to: UInt8.self)
            let rowBytes = CVPixelBufferGetBytesPerRow(buffer)
            for y in 0..<180 {
                for x in 0..<320 {
                    let white = x >= frame * 3 && x < frame * 3 + 24 && y >= 60 && y < 84
                    let i = y * rowBytes + x * 4
                    pixels[i] = white ? 240 : (blue ? 210 : 20)
                    pixels[i + 1] = white ? 240 : 35
                    pixels[i + 2] = white ? 240 : (blue ? 20 : 210)
                    pixels[i + 3] = 255
                }
            }
            CVPixelBufferUnlockBaseAddress(buffer, [])
            guard adapter.append(buffer, withPresentationTime: CMTime(value: Int64(frame), timescale: 24))
                else { throw writer.error! }
        }
        input.markAsFinished()
        await writer.finishWriting()
        guard writer.status == .completed else { throw writer.error! }
    }

    static func inspect(_ url: URL) async throws {
        let asset = AVURLAsset(url: url)
        let duration = try await asset.load(.duration)
        let video = try await asset.loadTracks(withMediaType: .video)
        let audio = try await asset.loadTracks(withMediaType: .audio)
        var tracks: [[String: Any]] = []
        for track in video {
            let size = try await track.load(.naturalSize)
            let fps = try await track.load(.nominalFrameRate)
            tracks.append(["width": size.width, "height": size.height, "fps": fps])
        }
        let result: [String: Any] = ["name": url.lastPathComponent,
            "duration_seconds": duration.seconds, "video": tracks, "audio_track_count": audio.count]
        print(String(data: try JSONSerialization.data(withJSONObject: result, options: [.sortedKeys]), encoding: .utf8)!)
    }

    static func audioProbe(_ asset: AVURLAsset) async throws -> [String: Any] {
        let track = try await asset.loadTracks(withMediaType: .audio).first!
        let reader = try AVAssetReader(asset: asset)
        let output = AVAssetReaderTrackOutput(track: track, outputSettings: [
            AVFormatIDKey: kAudioFormatLinearPCM, AVLinearPCMIsFloatKey: true,
            AVLinearPCMBitDepthKey: 32, AVLinearPCMIsNonInterleaved: false])
        reader.add(output)
        guard reader.startReading() else { throw reader.error! }
        var samples = 0, crossings = 0
        var squareSum = 0.0, previous = Float(0), rate = 0.0
        while let sample = output.copyNextSampleBuffer() {
            let format = CMSampleBufferGetFormatDescription(sample)!
            let asbd = CMAudioFormatDescriptionGetStreamBasicDescription(format)!.pointee
            rate = asbd.mSampleRate
            let channels = Int(asbd.mChannelsPerFrame)
            let buffer = CMSampleBufferGetDataBuffer(sample)!
            var data = Data(count: CMBlockBufferGetDataLength(buffer))
            let length = data.count
            let status = data.withUnsafeMutableBytes { bytes in
                CMBlockBufferCopyDataBytes(buffer, atOffset: 0, dataLength: length, destination: bytes.baseAddress!)
            }
            guard status == noErr else { throw NSError(domain: "audio.copy", code: Int(status)) }
            data.withUnsafeBytes { bytes in
                let values = bytes.bindMemory(to: Float.self)
                for i in stride(from: 0, to: values.count, by: channels) {
                    let value = values[i]
                    squareSum += Double(value * value)
                    if previous < 0 && value >= 0 { crossings += 1 }
                    previous = value
                    samples += 1
                }
            }
        }
        guard reader.status == .completed && samples > 0 else { throw NSError(domain: "audio.decode", code: 1) }
        return ["samples": samples, "sample_rate": rate,
                "rms": sqrt(squareSum / Double(samples)),
                "zero_crossing_hz": Double(crossings) * rate / Double(samples)]
    }

    static func verifyExport(_ url: URL) async throws {
        let asset = AVURLAsset(url: url)
        let generator = AVAssetImageGenerator(asset: asset)
        generator.requestedTimeToleranceBefore = .zero
        generator.requestedTimeToleranceAfter = .zero
        var frames: [[String: Any]] = []
        for second in [1.0, 3.0, 4.5] {
            let capture = try await generator.image(at: CMTime(seconds: second, preferredTimescale: 600))
            let image = capture.image
            let target = url.deletingLastPathComponent().appendingPathComponent("\(url.deletingPathExtension().lastPathComponent)-frame-\(second).png")
            let destination = CGImageDestinationCreateWithURL(target as CFURL, UTType.png.identifier as CFString, 1, nil)!
            CGImageDestinationAddImage(destination, image, nil)
            guard CGImageDestinationFinalize(destination) else { throw NSError(domain: "frame.write", code: 1) }
            var pixels = [UInt8](repeating: 0, count: image.width * image.height * 4)
            pixels.withUnsafeMutableBytes { bytes in
                let context = CGContext(data: bytes.baseAddress, width: image.width, height: image.height,
                    bitsPerComponent: 8, bytesPerRow: image.width * 4,
                    space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
                context.draw(image, in: CGRect(x: 0, y: 0, width: image.width, height: image.height))
            }
            let index = (10 * image.width + 10) * 4
            frames.append(["second": second, "actual_second": capture.actualTime.seconds,
                "name": target.lastPathComponent, "sample_rgb": Array(pixels[index..<index + 3])])
        }
        let result: [String: Any] = ["name": url.lastPathComponent, "frames": frames,
                                    "audio": try await audioProbe(asset)]
        print(String(data: try JSONSerialization.data(withJSONObject: result, options: [.sortedKeys]), encoding: .utf8)!)
    }

    static func main() async throws {
        let args = CommandLine.arguments
        guard args.count == 3 else { throw NSError(domain: "fixture.arguments", code: 3) }
        let url = URL(fileURLWithPath: args[2])
        if args[1] == "make" {
            try await makeClip(url.appendingPathComponent("red.mov"), blue: false)
            try await makeClip(url.appendingPathComponent("blue.mov"), blue: true)
            try await inspect(url.appendingPathComponent("red.mov"))
            try await inspect(url.appendingPathComponent("blue.mov"))
        } else if args[1] == "inspect" {
            try await inspect(url)
        } else if args[1] == "clip" {
            try await makeClip(url, blue: false)
            try await inspect(url)
        } else if args[1] == "verify-export" {
            try await verifyExport(url)
        } else if args[1] == "audio" {
            print(String(data: try JSONSerialization.data(withJSONObject: try await audioProbe(AVURLAsset(url: url)), options: [.sortedKeys]), encoding: .utf8)!)
        } else { throw NSError(domain: "fixture.operation", code: 4) }
    }
}

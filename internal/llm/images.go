package llm

// Pixel input limits are shared by working-set selection, authorized reads and
// the Host's provider envelope. Durable messages contain references only.
const (
	MaxInputImages      = 8
	MaxInputImageBytes  = 8 << 20
	MaxInputImagePixels = 32 << 20
)

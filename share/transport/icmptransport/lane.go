package icmptransport

const (
	StreamDefault uint32 = 0
	StreamControl uint32 = 1
	StreamData    uint32 = 2
)

type StreamWriter interface {
	WriteWithStream(streamID uint32, p []byte) (int, error)
}

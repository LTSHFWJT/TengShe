package share

import "io"

func CloseQuietly(closer io.Closer) {
	if closer != nil {
		_ = closer.Close()
	}
}

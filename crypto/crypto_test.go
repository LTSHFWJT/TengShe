package crypto

import (
	"bytes"
	"testing"
)

func TestKeyPadding(t *testing.T) {
	if got := KeyPadding(nil); got != nil {
		t.Fatalf("empty key padding = %v, want nil", got)
	}

	short := []byte("secret")
	padded := KeyPadding(short)
	if len(padded) != 32 {
		t.Fatalf("short key length = %d, want 32", len(padded))
	}
	if !bytes.Equal(padded[:len(short)], short) {
		t.Fatalf("short key prefix = %q, want %q", padded[:len(short)], short)
	}

	exact := []byte("12345678901234567890123456789012")
	if got := KeyPadding(exact); !bytes.Equal(got, exact) {
		t.Fatalf("exact key = %q, want %q", got, exact)
	}

	long := []byte("12345678901234567890123456789012-extra")
	if got := KeyPadding(long); !bytes.Equal(got, long[:32]) {
		t.Fatalf("long key = %q, want %q", got, long[:32])
	}
}

func TestAESRoundTrip(t *testing.T) {
	key := KeyPadding([]byte("secret"))
	plain := []byte("hello tengshe")

	encrypted := AESEncrypt(plain, key)
	if bytes.Equal(encrypted, plain) {
		t.Fatal("encrypted payload matches plaintext")
	}

	decrypted := AESDecrypt(encrypted, key)
	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("decrypted payload = %q, want %q", decrypted, plain)
	}
}

func TestAESWithEmptyKeyPassesThrough(t *testing.T) {
	plain := []byte("plain")

	if got := AESEncrypt(plain, nil); !bytes.Equal(got, plain) {
		t.Fatalf("nil-key encrypt = %q, want %q", got, plain)
	}
	if got := AESDecrypt(plain, nil); !bytes.Equal(got, plain) {
		t.Fatalf("nil-key decrypt = %q, want %q", got, plain)
	}
}

func TestGzipRoundTrip(t *testing.T) {
	plain := []byte("compressible compressible compressible")

	compressed := GzipCompress(plain)
	decompressed := GzipDecompress(compressed)

	if !bytes.Equal(decompressed, plain) {
		t.Fatalf("decompressed payload = %q, want %q", decompressed, plain)
	}
}

func TestGzipInvalidDataReturnsEmpty(t *testing.T) {
	if got := GzipDecompress([]byte("not gzip")); len(got) != 0 {
		t.Fatalf("invalid gzip result = %q, want empty", got)
	}
}

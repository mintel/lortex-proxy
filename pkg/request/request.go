package request

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"

	"github.com/valyala/bytebufferpool"
)

const minBufferSize = 32 * 1024 // This is what's used by io.Copy internally.

func Clone(ctx context.Context, req *http.Request) (*http.Request, *http.Request) {
	return clone2(ctx, req)
}

// clone1 fans out by reads the entire request body and creates two io.Readers from it.
func clone1(ctx context.Context, req *http.Request) (*http.Request, *http.Request) {
	req2 := req.Clone(ctx)
	// https://github.com/golang/go/issues/36095#issuecomment-568239806
	var b bytes.Buffer
	ioCopy(&b, req.Body)
	req.Body.Close()
	req.Body = io.NopCloser(&b)
	req2.Body = io.NopCloser(bytes.NewReader(b.Bytes()))
	return req, req2
}

// clone2 fans using io.Pipe.
func clone2(ctx context.Context, req *http.Request) (*http.Request, *http.Request) {
	originalBody := req.Body
	req2 := req.Clone(ctx)
	p1r, p1w := io.Pipe()
	p2r, p2w := io.Pipe()
	w := io.MultiWriter(p1w, p2w)
	go func() {
		defer originalBody.Close()
		defer p1w.Close()
		defer p2w.Close()
		if _, err := ioCopy(w, originalBody); err != nil {
			log.Panic(err)
		}
	}()
	req.Body = p1r
	req2.Body = p2r
	return req, req2
}

// ioCopy works like io.Copy, but with fewer memory allocations.
func ioCopy(dst io.Writer, src io.Reader) (written int64, err error) {
	buf := getByteBufferWithCap(minBufferSize)
	defer bytebufferpool.Put(buf)
	b := buf.B
	b = b[:cap(b)]
	return io.CopyBuffer(dst, src, b)
}

func getByteBufferWithCap(n int) *bytebufferpool.ByteBuffer {
	buf := bytebufferpool.Get()
	b := buf.B
	b = b[:cap(b)]
	if cap(b) < n {
		b = append(make([]byte, 0, n-cap(b)), b...)
		b = b[:cap(b)]
	}
	b = b[:0]
	buf.B = b
	return buf
}

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
	return clone1(ctx, req)
}

// clone1 fans out by reads the entire request body and creates two io.Readers from it.
func clone1(ctx context.Context, req *http.Request) (*http.Request, *http.Request) {
	req2 := req.Clone(ctx)
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			log.Printf("warn: error from request.GetBody: %s", err)
		} else {
			req2.Body = body
			req2.GetBody = req.GetBody
			return req, req2
		}
	}
	// https://github.com/golang/go/issues/36095#issuecomment-568239806
	var b bytes.Buffer
	ioCopy(&b, req.Body)
	req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(b.Bytes()))
	req2.Body = io.NopCloser(bytes.NewReader(b.Bytes()))
	getBody := func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(b.Bytes())), nil
	}
	req.GetBody = getBody
	req2.GetBody = getBody
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

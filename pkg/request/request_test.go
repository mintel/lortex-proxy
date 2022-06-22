package request

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/valyala/bytebufferpool"
)

func drainBody(r *http.Request) {
	defer r.Body.Close()
	io.Copy(io.Discard, r.Body)
}

func randData(n int) []byte {
	data := make([]byte, n)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return data
}

func benchmarkCloneFunc(b *testing.B, f func(ctx context.Context, req *http.Request) (*http.Request, *http.Request)) {
	// Warm up the byte buffers so early tests don't have a disadvantage.
	var buffers []*bytebufferpool.ByteBuffer
	for i := 0; i < b.N*10; i++ {
		buffers = append(buffers, getByteBufferWithCap(minBufferSize))
	}
	for _, buf := range buffers {
		bytebufferpool.Put(buf)
	}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r1, r2 := f(context.Background(), r)
		wg := sync.WaitGroup{}
		wg.Add(2)
		go func() {
			defer wg.Done()
			drainBody(r1)
		}()
		go func() {
			defer wg.Done()
			drainBody(r2)
		}()
		wg.Wait()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer s.Close()
	c := s.Client()

	b.Run("16kb", func(b *testing.B) {
		data := randData(16 * 1024)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Post(s.URL, "text/plain", bytes.NewReader(data))
		}
	})

	b.Run("32kb", func(b *testing.B) {
		data := randData(32 * 1024)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Post(s.URL, "text/plain", bytes.NewReader(data))
		}
	})

	b.Run("64kb", func(b *testing.B) {
		data := randData(64 * 1024)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Post(s.URL, "text/plain", bytes.NewReader(data))
		}
	})

	b.Run("128kb", func(b *testing.B) {
		data := randData(128 * 1024)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Post(s.URL, "text/plain", bytes.NewReader(data))
		}
	})
}

func Benchmark_clone1(b *testing.B) {
	benchmarkCloneFunc(b, clone1)
}

func Benchmark_clone2(b *testing.B) {
	benchmarkCloneFunc(b, clone2)
}

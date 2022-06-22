package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"

	"github.com/elazarl/goproxy"
)

var (
	listen        = flag.String("listen", ":12345", "Listen at this address.")
	upstream      = flag.String("upstream", "http://localhost:8080", "Proxy traffic to this upstream URL.")
	mirrorPattern = flag.String("mirror.pattern", `(https?)://([^/]+)(.*)`, "A regex pattern that matches on the original request URL.")
	mirrorReplace = flag.String("mirror.replace", "$1://$2$3", "Send to the mirror server by replacing the URL matched by -mirror.pattern with this. Allows regex substitution.")
	verbose       = flag.Bool("verbose", false, "Print debug information.")
)

var (
	mirrorRegex *regexp.Regexp

	upstreamURL      *url.URL
	upstreamDirector func(*http.Request)
)

func init() {
	flag.Parse()

	var err error

	upstreamURL, err = url.Parse(*upstream)
	if err != nil {
		log.Panic(err)
	}

	mirrorRegex = regexp.MustCompile(*mirrorPattern)

	upstreamDirector = httputil.NewSingleHostReverseProxy(upstreamURL).Director
}

type schemeWrapper struct {
	*goproxy.ProxyHttpServer
}

func (sw *schemeWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Scheme == "" {
		r.URL.Scheme = "http"
	}
	if r.URL.Host == "" && r.Host != "" {
		r.URL.Host = r.Host
	}
	sw.ProxyHttpServer.ServeHTTP(w, r)
}

func main() {
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = *verbose
	proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		go func(r *http.Request) {
			originalURL := r.URL.String()
			if *verbose {
				log.Printf("[%d] original url: %s\n", ctx.Session, originalURL)
			}

			newURL, err := url.Parse(mirrorRegex.ReplaceAllString(originalURL, *mirrorReplace))
			if err != nil {
				log.Panic(newURL)
			}
			if *verbose {
				log.Printf("[%d] new url: %s\n", ctx.Session, newURL.String())
			}

			r.URL = newURL
			r.Host = newURL.Host

			resp, err := http.DefaultTransport.RoundTrip(r)
			if err != nil {
				return
			}

			defer resp.Body.Close()
			if *verbose {
				buf := &bytes.Buffer{}
				buf.ReadFrom(resp.Body)
				log.Printf("[%d] response body: \n%s\n", ctx.Session, buf.Bytes())
			} else {
				_, _ = io.Copy(io.Discard, resp.Body)
			}
		}(req.Clone(context.Background()))

		if req.Host == "" {
			req.Host = req.URL.Host
		}
		upstreamDirector(req)

		return req, nil
	})
	if err := http.ListenAndServe(*listen, &schemeWrapper{proxy}); err != nil {
		log.Fatal(err)
	}
}
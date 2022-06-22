package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"

	"github.com/elazarl/goproxy"
	"github.com/valyala/bytebufferpool"
)

var (
	listen        = flag.String("listen", ":12345", "Listen at this address.")
	upstream      = flag.String("upstream", "http://localhost:8080", "Proxy traffic to this upstream URL.")
	mirrorPattern = flag.String("mirror.pattern", `^(https?)://([^/]+)(.*)$`, "A regex pattern that matches on the original request URL.")
	mirrorReplace = flag.String("mirror.replace", "$1://$2$3", "Send to the mirror server by replacing the URL matched by -mirror.pattern with this. Allows regex substitution.")
	mirrorIgnore  = flag.String("mirror.ignore", "^https?://[^/]+/(ready|readyz|healthy|healthz|external-health-check|metrics)$", "Ignore requests for URLs matching this regexp pattern.")
	verbose       = flag.Bool("verbose", false, "Print debug information.")
)

var (
	mirrorPatternRegex *regexp.Regexp
	mirrorIgnoreRegex  *regexp.Regexp

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

	mirrorPatternRegex = regexp.MustCompile(*mirrorPattern)
	mirrorIgnoreRegex = regexp.MustCompile(*mirrorIgnore)

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
		sendMirrorCopy := func(r *http.Request) {
			u := *r.URL
			u.RawQuery = ""
			originalURL := u.String()
			if *verbose {
				log.Printf("[%03d] DEBUG: request original url: %s\n", ctx.Session, originalURL)
			}

			newURL, err := url.Parse(mirrorPatternRegex.ReplaceAllString(originalURL, *mirrorReplace))
			if err != nil {
				log.Panic(newURL)
			}
			if *verbose {
				log.Printf("[%03d] DEBUG: new mirror url: %s\n", ctx.Session, newURL.String())
			}

			r.URL = newURL
			r.Host = newURL.Host

			resp, err := http.DefaultTransport.RoundTrip(r)
			if err != nil {
				return
			}

			defer resp.Body.Close()
			if *verbose {
				log.Printf("[%03d] DEBUG: received mirror response: %s\n", ctx.Session, resp.Status)
				buf := bytebufferpool.Get()
				defer bytebufferpool.Put(buf)
				_, err := buf.ReadFrom(resp.Body)
				if err != nil {
					log.Printf("[%03d] WARN: error reading mirror response body: \n%s\n", ctx.Session, err)
				} else {
					log.Printf("[%03d] DEBUG: mirror response body: \n%s\n", ctx.Session, buf.Bytes())
				}
			} else {
				_, _ = io.Copy(io.Discard, resp.Body)
			}
		}

		u := *req.URL
		u.RawQuery = ""
		if req.Host != "" {
			u.Host = req.Host
		} else if h := req.Header.Get("Host"); h != "" {
			u.Host = h
		}
		if !mirrorIgnoreRegex.MatchString(u.String()) {
			go sendMirrorCopy(req.Clone(context.Background()))
		} else if *verbose {
			log.Printf("[%03d] DEBUG: not sending request to mirror\n", ctx.Session)
		}

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

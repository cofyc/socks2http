package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"

	"github.com/cofyc/xhttproxy/pkg/version"
	"golang.org/x/net/proxy"
)

func handleTunneling(dialer proxy.Dialer, w http.ResponseWriter, r *http.Request) {
	if dialer == nil {
		dialer = proxy.Direct
	}
	dest_conn, err := dialer.Dial("tcp", r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	client_conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
	go transfer(dest_conn, client_conn)
	go transfer(client_conn, dest_conn)
}

func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
}

func handleHTTP(transport http.RoundTripper, w http.ResponseWriter, req *http.Request) {
	if transport == nil {
		transport = http.DefaultTransport
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

var (
	optPemPath string
	optKeyPath string
	optProto   string
	optAddress string
	optSocks   string
	optVersion bool
)

func init() {
	flag.StringVar(&optPemPath, "pem", "server.pem", "path to pem file")
	flag.StringVar(&optKeyPath, "key", "server.key", "path to key file")
	flag.StringVar(&optAddress, "addr", "0.0.0.0:8888", "address to listen (default: 0.0.0.0:8888)")
	flag.StringVar(&optProto, "proto", "https", "protocol to use (http or https, default: https)")
	flag.StringVar(&optSocks, "socks", "", "socks server to use")
	flag.BoolVar(&optVersion, "version", false, "show version")
}

// httpDialer wraps on proxy.Dialer to add DialContext method.
type httpDialer struct {
	proxy.Dialer
}

func (d *httpDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.Dial(network, addr)
}

func (d *httpDialer) Dial(network, addr string) (net.Conn, error) {
	return d.Dialer.Dial(network, addr)
}

func main() {
	flag.Parse()
	if optVersion {
		fmt.Printf("xhttproxy %s\n", version.VERSION)
		return
	}
	if optProto != "http" && optProto != "https" {
		log.Fatal("Protocol must be either http or https")
	}
	var dialer proxy.Dialer
	var transport http.RoundTripper
	var err error
	if optSocks != "" {
		dialer, err = proxy.SOCKS5("tcp", optSocks, nil, proxy.Direct)
		if err != nil {
			log.Fatal(err)
		}
		d := &httpDialer{dialer}
		transport = &http.Transport{
			DialContext: d.DialContext,
			Dial:        d.Dial,
		}
	}
	server := &http.Server{
		Addr: optAddress,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect {
				handleTunneling(dialer, w, r)
			} else {
				handleHTTP(transport, w, r)
			}
		}),
		// Disable HTTP/2.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
	if optProto == "http" {
		log.Fatal(server.ListenAndServe())
	} else {
		log.Fatal(server.ListenAndServeTLS(optPemPath, optKeyPath))
	}
}

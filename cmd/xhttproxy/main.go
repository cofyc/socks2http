package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/cofyc/xhttproxy/pkg/version"
	"github.com/golang/glog"
	"golang.org/x/net/proxy"
)

func handleTunneling(dialer proxy.Dialer, w http.ResponseWriter, r *http.Request) {
	if dialer == nil {
		dialer = proxy.Direct
	}
	glog.V(4).Infof("dialing %s", r.Host)
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

func handleHTTP(transport http.RoundTripper, w http.ResponseWriter, r *http.Request) {
	glog.V(4).Infof("roundtrip to %s", r.Host)
	if transport == nil {
		transport = http.DefaultTransport
	}
	resp, err := transport.RoundTrip(r)
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
	// We log to stderr because glog will default to logging to a file.
	flag.Set("logtostderr", "true")
	flag.Parse()
	if optVersion {
		fmt.Printf("xhttproxy %s\n", version.VERSION)
		return
	}
	if optProto != "http" && optProto != "https" {
		glog.Fatal("Protocol must be either http or https")
	}
	var dialer proxy.Dialer
	var transport http.RoundTripper
	var err error
	if optSocks != "" {
		dialer, err = proxy.SOCKS5("tcp", optSocks, nil, proxy.Direct)
		if err != nil {
			glog.Fatal(err)
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
	glog.Infof("listen %s on %s", optProto, optAddress)
	if optProto == "http" {
		glog.Fatal(server.ListenAndServe())
	} else {
		glog.Fatal(server.ListenAndServeTLS(optPemPath, optKeyPath))
	}
}

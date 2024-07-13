package unpackerr

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"path"
	"strings"

	"github.com/julienschmidt/httprouter"
	apachelog "github.com/lestrrat-go/apache-logformat/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type WebServer struct {
	Metrics    bool        `json:"metrics"     toml:"metrics"       xml:"metrics"       yaml:"metrics"`
	LogFiles   int         `json:"logFiles"    toml:"log_files"     xml:"log_files"     yaml:"logFiles"`
	LogFileMb  int         `json:"logFileMb"   toml:"log_file_mb"   xml:"log_file_mb"   yaml:"logFileMb"`
	ListenAddr string      `json:"listenAddr"  toml:"listen_addr"   xml:"listen_addr"   yaml:"listenAddr"`
	LogFile    string      `json:"logFile"     toml:"log_file"      xml:"log_file"      yaml:"logFile"`
	SSLCrtFile string      `json:"sslCertFile" toml:"ssl_cert_file" xml:"ssl_cert_file" yaml:"sslCertFile"`
	SSLKeyFile string      `json:"sslKeyFile"  toml:"ssl_key_file"  xml:"ssl_key_file"  yaml:"sslKeyFile"`
	URLBase    string      `json:"urlbase"     toml:"urlbase"       xml:"urlbase"       yaml:"urlbase"`
	Upstreams  StringSlice `json:"upstreams"   toml:"upstreams"     xml:"upstreams"     yaml:"upstreams"`
	allow      AllowedIPs
	router     *httprouter.Router
	server     *http.Server
}

func (w *WebServer) Enabled() bool {
	return w != nil && w.Metrics && w.ListenAddr != ""
}

func (u *Unpackerr) logWebserver() {
	if !u.Webserver.Enabled() {
		u.Printf(" => Webserver Disabled")
		return
	}

	addr := u.Webserver.ListenAddr
	if !strings.Contains(addr, ":") {
		addr = "0.0.0.0:" + addr
	}

	ssl := ""
	if u.Webserver.SSLCrtFile != "" && u.Webserver.SSLKeyFile != "" {
		ssl = "s"
	}

	u.Printf(" => Starting webserver. Listen address: http%s://%v%s (%d upstreams)",
		ssl, addr, u.Webserver.URLBase, len(u.Webserver.Upstreams))
}

func (u *Unpackerr) startWebServer() {
	if !u.Webserver.Enabled() {
		return
	}

	addr := u.Webserver.ListenAddr
	if !strings.Contains(addr, ":") {
		addr = "0.0.0.0:" + addr
	}

	u.Webserver.URLBase = strings.TrimSuffix(path.Join("/", u.Webserver.URLBase), "/") + "/"
	u.Webserver.allow = MakeIPs(u.Webserver.Upstreams)
	u.Webserver.router = httprouter.New()
	apache, _ := apachelog.New(`%{X-Forwarded-For}i %l - %t "%r" %>s %b "%{Referer}i" "%{User-agent}i"`)

	// Make a multiplexer because websockets can't use apache log.
	smx := http.NewServeMux()
	smx.Handle(path.Join(u.Webserver.URLBase, "ws"), u.fixForwardedFor(u.Webserver.router))
	smx.Handle("/", u.fixForwardedFor(apache.Wrap(u.Webserver.router, u.Logger.HTTP.Writer())))
	u.webRoutes()

	u.Webserver.server = &http.Server{
		Addr:              addr,
		Handler:           smx,
		ReadTimeout:       0,
		ReadHeaderTimeout: defaultTimeout,
		WriteTimeout:      0,
		IdleTimeout:       defaultTimeout,
		ErrorLog:          u.Logger.Error,
	}

	go u.runWebServer()
}

func (u *Unpackerr) webRoutes() {
	u.Webserver.router.GET(path.Join(u.Webserver.URLBase, "/"), Index)

	if !u.Webserver.Metrics {
		return
	}

	u.setupMetrics()
	u.Webserver.router.Handler(http.MethodGet, "/metrics", promhttp.Handler())

	if u.Webserver.URLBase != "/" {
		// Metrics get served from both paths.
		u.Webserver.router.Handler(http.MethodGet, path.Join(u.Webserver.URLBase, "/metrics"), promhttp.Handler())
	}
}

// runWebServer starts the http or https listener.
func (u *Unpackerr) runWebServer() {
	var err error

	if u.Webserver.SSLCrtFile != "" && u.Webserver.SSLKeyFile != "" {
		err = u.Webserver.server.ListenAndServeTLS(
			expandHomedir(u.Webserver.SSLCrtFile), expandHomedir(u.Webserver.SSLKeyFile))
	} else {
		err = u.Webserver.server.ListenAndServe()
	}

	if err != nil && !errors.Is(http.ErrServerClosed, err) {
		u.Errorf("Web Server Failed: %v", err)
	}
}

func Index(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, "Welcome!\n")
}

// fixForwardedFor sets the X-Forwarded-For header to the client IP
// under specific circumstances.
func (u *Unpackerr) fixForwardedFor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { //nolint:varnamelen
		if x := r.Header.Get("X-Forwarded-For"); x == "" || !u.Webserver.allow.Contains(r.RemoteAddr) {
			r.Header.Set("X-Forwarded-For",
				strings.Trim(r.RemoteAddr[:strings.LastIndex(r.RemoteAddr, ":")], "[]"))
		} else if l := strings.LastIndexAny(x, ", "); l != -1 {
			r.Header.Set("X-Forwarded-For", strings.Trim(x[l:len(x)-1], ", "))
		}

		next.ServeHTTP(w, r)
	})
}

/* This is a helper method to check if an IP is in a list/cidr. */

// AllowedIPs determines who can set x-forwarded-for.
type AllowedIPs struct {
	Input []string
	Nets  []*net.IPNet
}

var _ = fmt.Stringer(AllowedIPs{})

// String turns a list of allowedIPs into a printable masterpiece.
func (n AllowedIPs) String() (s string) {
	if len(n.Nets) < 1 {
		return "(none)"
	}

	for i := range n.Nets {
		if s != "" {
			s += ", "
		}

		s += n.Nets[i].String()
	}

	return s
}

// Contains returns true if an IP is allowed.
func (n AllowedIPs) Contains(ip string) bool {
	ip = strings.Trim(ip[:strings.LastIndex(ip, ":")], "[]")

	for i := range n.Nets {
		if n.Nets[i].Contains(net.ParseIP(ip)) {
			return true
		}
	}

	return false
}

// MakeIPs turns a list of CIDR strings (or plain IPs) into a list of net.IPNet.
// This "allowed" list is later used to check incoming IPs from web requests.
func MakeIPs(upstreams []string) AllowedIPs {
	a := AllowedIPs{
		Input: make([]string, len(upstreams)),
		Nets:  []*net.IPNet{},
	}

	for idx, ipAddr := range upstreams {
		a.Input[idx] = ipAddr

		if !strings.Contains(ipAddr, "/") {
			if strings.Contains(ipAddr, ":") {
				ipAddr += "/128"
			} else {
				ipAddr += "/32"
			}
		}

		if _, i, err := net.ParseCIDR(ipAddr); err == nil {
			a.Nets = append(a.Nets, i)
		}
	}

	return a
}

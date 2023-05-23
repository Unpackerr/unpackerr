package unpackerr

import (
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	apachelog "github.com/lestrrat-go/apache-logformat/v2"
)

const webserverTimeout = 10 * time.Second

type ServerConfig struct {
	Metrics    bool   `toml:"metrics" json:"metrics" xml:"metrics" yaml:"metrics"`
	ListenAddr string `toml:"listen_addr" json:"listenAddr" xml:"listen_addr" yaml:"listenAddr"`
	LogFile    string `json:"logFile" toml:"log_file" xml:"log_file" yaml:"logFile"`
	LogFiles   int    `json:"logFiles" toml:"log_files" xml:"log_files" yaml:"logFiles"`
	LogFileMb  int    `json:"logFileMb" toml:"log_file_mb" xml:"log_file_mb" yaml:"logFileMb"`
	URLBase    string
	router     *httprouter.Router
}

func (u *Unpackerr) logWebserver() {}

func (u *Unpackerr) startWebServer() error {
	if u.Webserver == nil || !u.Webserver.Metrics || u.Webserver.ListenAddr == "" {
		return nil
	}

	addr := u.Webserver.ListenAddr
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}

	u.Webserver.router = httprouter.New()
	apache, _ := apachelog.New(`%{X-Forwarded-For}i %l %{X-NotiClient-Username}i %t "%m %{X-Redacted-URI}i %H" %>s %b` +
		` "%{Referer}i" "%{User-agent}i" %{X-Request-Time}i %{ms}Tms`)

	// Make a multiplexer because websockets can't use apache log.
	smx := http.NewServeMux()
	smx.Handle(path.Join(u.Webserver.URLBase, "ws"), u.Webserver.router) // websockets cannot go through the apache logger.
	smx.Handle("/", apache.Wrap(u.Webserver.router, u.Logger.HTTP.Writer()))

	u.Webserver.router.GET("/", Index)

	srv := &http.Server{
		Addr:              addr,
		Handler:           u.Webserver.router,
		ReadTimeout:       webserverTimeout,
		ReadHeaderTimeout: webserverTimeout,
		WriteTimeout:      webserverTimeout,
		IdleTimeout:       webserverTimeout,
		ErrorLog:          u.Logger.Error,
	}

	return srv.ListenAndServe()
}

func Index(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, "Welcome!\n")
}

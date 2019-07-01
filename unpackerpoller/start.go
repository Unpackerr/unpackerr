package unpackerpoller

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golift/deluge"

	"github.com/naoina/toml"
	flg "github.com/ogier/pflag"
	"github.com/pkg/errors"
)

const (
	defaultConfFile    = "/etc/unpacker-poller/up.conf"
	minimumInterval    = 1 * time.Minute
	minimumDeleteDelay = 1 * time.Minute
	defaultTimeout     = 10 * time.Second
)

// Version of the application. Injected at build time.
var Version = "development"

// Start runs the app.
func Start() error {
	u := &UnpackerPoller{
		History:  &History{Map: make(map[string]Extracts)},
		StopChan: make(chan os.Signal, 0),
	}
	u.ParseFlags()
	if u.verReq {
		fmt.Println("unpacker-poller version:", Version)
		return nil // don't run anything else.
	}
	log.Printf("Unpacker Poller Starting! (PID: %v)", os.Getpid())
	err := u.GetConfig()
	if err != nil {
		return errors.Wrap(err, "config")
	}
	u.Deluge, err = deluge.New(*u.Config.Deluge)
	if err != nil {
		return errors.Wrap(err, "deluge")
	}
	if u.Debug {
		u.Deluge.DebugLog = log.Printf
	}
	u.Run()
	return nil
}

// Run starts the go routines and waits for an exit signal.
func (u *UnpackerPoller) Run() {
	// Run all pollers once at startup.
	u.pollAllApps()
	go u.PollChange() // This has its own ticker that runs every minute.
	go func() {
		ticker := time.NewTicker(u.Interval.Duration)
		for range ticker.C {
			u.pollAllApps()
		}
	}()
	signal.Notify(u.StopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	log.Println("\nExiting! Caught Signal:", <-u.StopChan)
}

// ParseFlags turns CLI args into usable data.
func (u *UnpackerPoller) ParseFlags() {
	flg.Usage = func() {
		fmt.Println("Usage: unpacker-poller [--config=filepath] [--debug] [--version]")
		flg.PrintDefaults()
	}
	flg.StringVarP(&u.ConfigFile, "config", "c", defaultConfFile, "Poller Config File (TOML Format)")
	flg.BoolVarP(&u.Debug, "debug", "D", false, "Turn on the Spam (default false)")
	flg.BoolVarP(&u.verReq, "version", "v", false, "Print the version and exit.")
	flg.Parse()
	if log.SetFlags(log.LstdFlags); u.Debug {
		log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
	}
}

// GetConfig parses and returns our configuration data.
func (u *UnpackerPoller) GetConfig() error {
	// Preload our defaults.
	u.Config = &Config{}
	u.DeLogf("Reading Config File: %v", u.Flags.ConfigFile)
	if buf, err := ioutil.ReadFile(u.Flags.ConfigFile); err != nil {
		return err
		// This is where the defaults in the config variable are overwritten.
	} else if err := toml.Unmarshal(buf, &u.Config); err != nil {
		return errors.Wrap(err, "invalid config")
	}
	return u.ValidateConfig()
}

// ValidateConfig makes sure config values are ok.
func (u *UnpackerPoller) ValidateConfig() error {
	if u.DeleteDelay.Duration < minimumDeleteDelay {
		u.DeLogf("Setting Minimum Delete Delay: %v", minimumDeleteDelay.String())
		u.DeleteDelay.Duration = minimumDeleteDelay
	}
	if u.ConcurrentExtracts < 1 {
		u.ConcurrentExtracts = 1
	} else if u.ConcurrentExtracts > 10 {
		u.ConcurrentExtracts = 10
	}
	u.DeLogf("Maximum Concurrent Extractions: %d", u.ConcurrentExtracts)
	// Fix up intervals.
	if u.Timeout.Duration == 0 {
		u.DeLogf("Setting Default Timeout: %v", defaultTimeout.String())
		u.Timeout.Duration = defaultTimeout
	}
	if u.Config.Deluge.Timeout.Duration == 0 {
		u.Config.Deluge.Timeout.Duration = u.Timeout.Duration
	}
	if u.Radarr.Timeout.Duration == 0 {
		u.Radarr.Timeout.Duration = u.Timeout.Duration
	}
	if u.Sonarr.Timeout.Duration == 0 {
		u.Sonarr.Timeout.Duration = u.Timeout.Duration
	}

	if u.Interval.Duration < minimumInterval {
		u.DeLogf("Setting Minimum Interval: %v", minimumInterval.String())
		u.Interval.Duration = minimumInterval
	}
	return nil
}

// Poll Deluge, Sonarr and Radarr. All at the same time.
func (u *UnpackerPoller) pollAllApps() {
	if u.Sonarr.APIKey != "" {
		go u.PollSonarr()
	}
	if u.Radarr.APIKey != "" {
		go u.PollRadarr()
	}
	if err := u.PollDeluge(); err != nil {
		// We got an error polling deluge, try to reconnect.
		u.Deluge, err = deluge.New(*u.Config.Deluge)
		if err != nil {
			log.Println("Deluge Authentication Error:", err)
			// When auth fails > 1 time while running, just exit. Only exit if things are not pending.
			// if r.eCount().extracting == 0 && r.eCount().extracted == 0 &&
			// r.eCount().imported == 0 && r.eCount().queued == 0 {
			// 	os.Exit(2)
			// }
			return
		}
	}
}

// DeLogf writes Debug log lines.
func (u *UnpackerPoller) DeLogf(msg string, v ...interface{}) {
	if u.Debug {
		log.Printf("[DEBUG] "+msg, v...)
	}
}

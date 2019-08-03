package unpackerpoller

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golift.io/deluge"
	"golift.io/starr"

	"github.com/BurntSushi/toml"
	flg "github.com/spf13/pflag"
)

const (
	defaultConfFile    = "/etc/unpacker-poller/up.conf"
	minimumInterval    = 10 * time.Second
	minimumDeleteDelay = 1 * time.Second
	defaultTimeout     = 10 * time.Second
)

// Version of the application. Injected at build time.
var Version = "development"

// New returns an UnpackerPoller struct full of defaults.
// An empty struct will surely cause you pain, so use this!
func New() *UnpackerPoller {
	u := &UnpackerPoller{
		Flags: &Flags{ConfigFile: defaultConfFile},
		Config: &Config{
			Timeout: starr.Duration{Duration: defaultTimeout},
			Radarr:  &starr.Config{Timeout: starr.Duration{Duration: defaultTimeout}},
			Sonarr:  &starr.Config{Timeout: starr.Duration{Duration: defaultTimeout}},
			Lidarr:  &starr.Config{Timeout: starr.Duration{Duration: defaultTimeout}},
			Deluge:  &deluge.Config{Timeout: deluge.Duration{Duration: defaultTimeout}},
		},
		Xfers:   &Xfers{Map: make(map[string]*deluge.XferStatusCompat)},
		SonarrQ: &SonarrQ{List: []*starr.SonarQueue{}},
		RadarrQ: &RadarrQ{List: []*starr.RadarQueue{}},
		History: &History{Map: make(map[string]Extracts)},
		Deluge:  &deluge.Deluge{},
		SigChan: make(chan os.Signal),
	}
	u.Config.Deluge.DebugLog = u.DeLogf
	return u
}

// Start runs the app.
func Start() (err error) {
	log.SetFlags(log.LstdFlags)
	u := New().ParseFlags()
	if u.Flags.verReq {
		fmt.Printf("unpacker-poller v%s\n", Version)
		return nil // don't run anything else.
	}
	log.Printf("Unpacker Poller Starting! (PID: %v)", os.Getpid())
	if _, err := u.ParseConfig(); err != nil {
		return err
	}
	u.validateConfig()
	if u.Debug {
		log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
	}
	if u.Deluge, err = deluge.New(*u.Config.Deluge); err != nil {
		return err
	}
	go u.Run()
	defer u.Stop()
	signal.Notify(u.SigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	log.Println("=====> Exiting! Caught Signal:", <-u.SigChan)
	return nil
}

// Run starts the go routines and waits for an exit signal.
func (u *UnpackerPoller) Run() {
	if u.StopChan != nil {
		// one poller wont run twice unless you get creative.
		// just make a second one if you want to poller moar.
		return
	}
	u.StopChan = make(chan bool)
	go u.PollChange() // This has its own ticker that runs every minute.
	u.PollAllApps()   // Run all pollers once at startup.
	ticker := time.NewTicker(u.Interval.Duration)
	for {
		select {
		case <-ticker.C:
			u.PollAllApps()
		case <-u.StopChan:
			return
		}
	}
}

// Stop brings the go routines to a halt.
func (u *UnpackerPoller) Stop() {
	if u.StopChan != nil {
		close(u.StopChan)
	}
	// Arbitrary, just give the two routines time to bail.
	// This wont work if they're in the middle of something.. oh well.
	time.Sleep(100 * time.Millisecond)
}

// ParseFlags turns CLI args into usable data.
func (u *UnpackerPoller) ParseFlags() *UnpackerPoller {
	flg.Usage = func() {
		fmt.Println("Usage: unpacker-poller [--config=filepath] [--version]")
		flg.PrintDefaults()
	}
	flg.StringVarP(&u.Flags.ConfigFile, "config", "c", defaultConfFile, "Poller Config File (TOML Format)")
	flg.BoolVarP(&u.Flags.verReq, "version", "v", false, "Print the version and exit.")
	flg.Parse()
	return u // so you can chain into ParseConfig.
}

// ParseConfig parses and returns our configuration data.
func (u *UnpackerPoller) ParseConfig() (*UnpackerPoller, error) {
	log.Printf("Reading Config File: %v", u.ConfigFile)
	if buf, err := ioutil.ReadFile(u.ConfigFile); err != nil {
		return u, err
		// This is where the defaults in the config variable are overwritten.
	} else if err := toml.Unmarshal(buf, &u.Config); err != nil {
		return u, err
	}
	return u, nil
}

// validateConfig makes sure config file values are ok.
func (u *UnpackerPoller) validateConfig() {
	if u.DeleteDelay.Duration < minimumDeleteDelay {
		u.DeleteDelay.Duration = minimumDeleteDelay
	}
	u.DeLogf("Minimum Delete Delay: %v", minimumDeleteDelay.String())
	if u.ConcurrentExtracts < 1 {
		u.ConcurrentExtracts = 1
	}
	u.DeLogf("Maximum Concurrent Extractions: %d", u.ConcurrentExtracts)
	if u.Interval.Duration < minimumInterval {
		u.Interval.Duration = minimumInterval
	}
	u.DeLogf("Minimum Interval: %v", minimumInterval.String())
}

// PollAllApps Polls Deluge, Sonarr and Radarr. All at the same time.
func (u *UnpackerPoller) PollAllApps() {
	var wg sync.WaitGroup
	if u.Sonarr.APIKey != "" {
		wg.Add(1)
		go func() {
			if err := u.PollSonarr(); err != nil {
				log.Printf("[ERROR] Sonarr: %v", err)
			}
			wg.Done()
		}()
	}
	if u.Radarr.APIKey != "" {
		wg.Add(1)
		go func() {
			if err := u.PollRadarr(); err != nil {
				log.Printf("[ERROR] Radarr: %v", err)
			}
			wg.Done()
		}()
	}
	wg.Add(1)
	go func() {
		if err := u.PollDeluge(); err != nil {
			log.Printf("[ERROR] Deluge: %v", err)
			// We got an error polling deluge, try to reconnect.
			if u.Deluge, err = deluge.New(*u.Config.Deluge); err != nil {
				log.Printf("Deluge Authentication Error: %v", err)
			}
		}
		wg.Done()
	}()
	wg.Wait()
}

// DeLogf writes Debug log lines.
func (u *UnpackerPoller) DeLogf(msg string, v ...interface{}) {
	if u.Debug {
		log.Printf("[DEBUG] "+msg, v...)
	}
}

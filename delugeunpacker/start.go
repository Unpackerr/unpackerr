package delugeunpacker

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golift.io/cnfg"
	"golift.io/cnfg/cnfgfile"
	"golift.io/deluge"
	"golift.io/starr"

	"github.com/prometheus/common/version"
	flg "github.com/spf13/pflag"
)

const (
	defaultConfFile    = "/etc/deluge-unpacker/du.conf"
	minimumInterval    = 10 * time.Second
	minimumDeleteDelay = 1 * time.Second
	defaultTimeout     = 20 * time.Second
	reconnectWait      = 5 * time.Second
)

// New returns an UnpackerPoller struct full of defaults.
// An empty struct will surely cause you pain, so use this!
func New() *UnpackerPoller {
	u := &UnpackerPoller{
		Flags: &Flags{ConfigFile: defaultConfFile},
		Config: &Config{
			Timeout: cnfg.Duration{Duration: defaultTimeout},
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
		fmt.Printf("deluge-unpacker v%s %s (branch: %s %s) \n",
			version.Version, version.BuildDate, version.Branch, version.Revision)
		return nil // don't run anything else.
	}

	log.Printf("Unpacker Poller v%s Starting! (PID: %v)", version.Version, os.Getpid())

	if err := cnfgfile.Unmarshal(&u.Config, u.ConfigFile); err != nil {
		return err
	}

	if _, err := cnfg.UnmarshalENV(&u.Config, "UP"); err != nil {
		return err
	}

	u.validateConfig()

	if u.Debug {
		log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
	}

	u.Deluge = GetDelugeConnection(u.Config.Deluge)

	go u.Run()
	defer u.Stop()
	signal.Notify(u.SigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	log.Println("=====> Exiting! Caught Signal:", <-u.SigChan)

	return nil
}

// GetDelugeConnection keeps trying to make a connection to Deluge every 5 seconds.
func GetDelugeConnection(config *deluge.Config) *deluge.Deluge {
	for {
		d, err := deluge.New(*config)
		if err != nil {
			log.Println("[ERROR] connecting to Deluge, still trying!", err)
			time.Sleep(reconnectWait)

			continue
		}

		return d
	}
}

// Run starts the go routines and waits for an exit signal.
func (u *UnpackerPoller) Run() {
	if u.StopChan != nil {
		// one poller wont run twice unless you get creative.
		// just make a second one if you want to poller moar.
		return
	}

	u.StopChan = make(chan bool)
	ticker := time.NewTicker(u.Interval.Duration)

	go u.PollChange() // This has its own ticker that runs every minute.
	u.PollAllApps()   // Run all pollers once at startup.

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
		fmt.Println("Usage: deluge-unpacker [--config=filepath] [--version]")
		flg.PrintDefaults()
	}

	flg.StringVarP(&u.Flags.ConfigFile, "config", "c", defaultConfFile, "Poller Config File (TOML Format)")
	flg.BoolVarP(&u.Flags.verReq, "version", "v", false, "Print the version and exit.")
	flg.Parse()

	return u // so you can chain into ParseConfig.
}

// validateConfig makes sure config file values are ok.
func (u *UnpackerPoller) validateConfig() {
	if u.DeleteDelay.Duration < minimumDeleteDelay {
		u.DeleteDelay.Duration = minimumDeleteDelay
		u.DeLogf("Minimum Delete Delay: %v", minimumDeleteDelay.String())
	}

	if u.ConcurrentExtracts < 1 {
		u.ConcurrentExtracts = 1
		u.DeLogf("Maximum Concurrent Extractions: %d", u.ConcurrentExtracts)
	}

	if u.Interval.Duration < minimumInterval {
		u.Interval.Duration = minimumInterval
		u.DeLogf("Minimum Interval: %v", minimumInterval.String())
	}
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
		_ = log.Output(2, fmt.Sprintf("[DEBUG] "+msg, v...))
	}
}

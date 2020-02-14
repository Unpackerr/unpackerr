package unpacker

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golift.io/cnfg"
	"golift.io/cnfg/cnfgfile"

	"github.com/prometheus/common/version"
	flg "github.com/spf13/pflag"
)

const (
	defaultTimeout     = 10 * time.Second
	minimumInterval    = 10 * time.Second
	defaultRetryDelay  = 5 * time.Minute
	defaultStartDelay  = time.Minute
	minimumDeleteDelay = time.Second
)

// New returns an UnpackerPoller struct full of defaults.
// An empty struct will surely cause you pain, so use this!
func New() *Unpackerr {
	return &Unpackerr{
		Flags:   &Flags{ConfigFile: defaultConfFile},
		SigChan: make(chan os.Signal),
		History: &History{Map: make(map[string]*Extracts)},
		Config: &Config{
			Timeout:     cnfg.Duration{Duration: defaultTimeout},
			Interval:    cnfg.Duration{Duration: minimumInterval},
			RetryDelay:  cnfg.Duration{Duration: defaultRetryDelay},
			StartDelay:  cnfg.Duration{Duration: defaultStartDelay},
			DeleteDelay: cnfg.Duration{Duration: minimumDeleteDelay},
		},
	}
}

// Start runs the app.
func Start() (err error) {
	log.SetFlags(log.LstdFlags)

	u := New().ParseFlags()
	if u.Flags.verReq {
		fmt.Printf("unpackerr v%s %s (branch: %s %s) \n",
			version.Version, version.BuildDate, version.Branch, version.Revision)
		return nil // don't run anything else.
	}

	log.Printf("[INFO] Unpackerr v%s Starting! (PID: %v)", version.Version, os.Getpid())

	if err := cnfgfile.Unmarshal(u.Config, u.ConfigFile); err != nil {
		return err
	}

	if _, err := cnfg.UnmarshalENV(u.Config, "UN"); err != nil {
		return err
	}

	u.validateConfig()

	if u.Debug {
		log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
	}

	go u.Run()
	signal.Notify(u.SigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	log.Println("=====> Exiting! Caught Signal:", <-u.SigChan)

	return nil
}

// Run starts the go routines and waits for an exit signal.
// One poller wont run twice unless you get creative.
// Just make a second one if you want to poller moar.
func (u *Unpackerr) Run() {
	u.DeLogf("Starting Cleanup Routine (interval: 1 minute)")

	poller := time.NewTicker(u.Interval.Duration)
	cleaner := time.NewTicker(time.Minute)

	go func() {
		for range cleaner.C {
			u.CheckExtractDone()
			u.CheckSonarrQueue()
			u.CheckRadarrQueue()
			u.CheckLidarrQueue()
		}
	}()
	go u.PollFolders()
	u.PollAllApps() // Run all pollers once at startup.

	for range poller.C {
		u.PollAllApps()
	}
}

// ParseFlags turns CLI args into usable data.
func (u *Unpackerr) ParseFlags() *Unpackerr {
	flg.Usage = func() {
		fmt.Println("Usage: unpackerr [--config=filepath] [--version]")
		flg.PrintDefaults()
	}

	flg.StringVarP(&u.Flags.ConfigFile, "config", "c", defaultConfFile, "Poller Config File (TOML Format)")
	flg.BoolVarP(&u.Flags.verReq, "version", "v", false, "Print the version and exit.")
	flg.Parse()

	return u // so you can chain into ParseConfig.
}

// validateConfig makes sure config file values are ok.
func (u *Unpackerr) validateConfig() {
	if u.DeleteDelay.Duration < minimumDeleteDelay {
		u.DeleteDelay.Duration = minimumDeleteDelay
		u.DeLogf("Minimum Delete Delay: %v", minimumDeleteDelay.String())
	}

	if u.Parallel < 1 {
		u.Parallel = 1
	}

	if u.Interval.Duration < minimumInterval {
		u.Interval.Duration = minimumInterval
		u.DeLogf("Minimum Interval: %v", minimumInterval.String())
	}

	for i := range u.Radarr {
		if u.Radarr[i].Timeout.Duration == 0 {
			u.Radarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Radarr[i].Path == "" {
			u.Radarr[i].Path = defaultSavePath
		}
	}

	for i := range u.Sonarr {
		if u.Sonarr[i].Timeout.Duration == 0 {
			u.Sonarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Sonarr[i].Path == "" {
			u.Sonarr[i].Path = defaultSavePath
		}
	}

	for i := range u.Lidarr {
		if u.Lidarr[i].Timeout.Duration == 0 {
			u.Lidarr[i].Timeout.Duration = u.Timeout.Duration
		}
	}
}

// DeLogf writes Debug log lines.
func (u *Unpackerr) DeLogf(msg string, v ...interface{}) {
	if u.Debug {
		_ = log.Output(2, fmt.Sprintf("[DEBUG] "+msg, v...))
	}
}

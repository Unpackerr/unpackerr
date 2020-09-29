package unpacker

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/common/version"
	flag "github.com/spf13/pflag"
	"golift.io/cnfg"
	"golift.io/cnfg/cnfgfile"
	"golift.io/xtractr"
)

// New returns an UnpackerPoller struct full of defaults.
// An empty struct will surely cause you pain, so use this!
func New() *Unpackerr {
	return &Unpackerr{
		Flags:   &Flags{ConfigFile: defaultConfFile},
		sigChan: make(chan os.Signal),
		History: &History{Map: make(map[string]*Extracts)},
		updates: make(chan *Extracts),
		Config: &Config{
			Timeout:     cnfg.Duration{Duration: defaultTimeout},
			Interval:    cnfg.Duration{Duration: minimumInterval},
			RetryDelay:  cnfg.Duration{Duration: defaultRetryDelay},
			StartDelay:  cnfg.Duration{Duration: defaultStartDelay},
			DeleteDelay: cnfg.Duration{Duration: minimumDeleteDelay},
		},
		log: log.New(ioutil.Discard, "", 0),
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

	if err := cnfgfile.Unmarshal(u.Config, u.ConfigFile); err != nil {
		return fmt.Errorf("config file: %w", err)
	}

	if _, err := cnfg.UnmarshalENV(u.Config, u.Flags.EnvPrefix); err != nil {
		return fmt.Errorf("environment variables: %w", err)
	}

	if err := u.setupLogging(); err != nil {
		return fmt.Errorf("log_file: %w", err)
	}

	u.Logf("Unpackerr v%s Starting! (PID: %v) %v", version.Version, os.Getpid(), time.Now())

	u.validateConfig()
	u.logStartupInfo()

	u.Xtractr = xtractr.NewQueue(&xtractr.Config{
		Debug:    u.Config.Debug,
		Parallel: int(u.Parallel),
		Suffix:   suffix,
		Logger:   u.log,
	})

	u.PollFolders() // this initializes channel(s) used in u.Run()

	go u.Run()
	signal.Notify(u.sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	u.Log("=====> Exiting! Caught Signal:", <-u.sigChan)

	return nil
}

// ParseFlags turns CLI args into usable data.
func (u *Unpackerr) ParseFlags() *Unpackerr {
	flag.Usage = func() {
		fmt.Println("Usage: unpackerr [--config=filepath] [--version]")
		flag.PrintDefaults()
	}

	flag.StringVarP(&u.Flags.ConfigFile, "config", "c", defaultConfFile, "Poller Config File (TOML Format)")
	flag.StringVarP(&u.Flags.EnvPrefix, "prefix", "p", "UN", "Environment Variable Prefix")
	flag.BoolVarP(&u.Flags.verReq, "version", "v", false, "Print the version and exit.")
	flag.Parse()

	return u // so you can chain into ParseConfig.
}

// validateConfig makes sure config file values are ok.
func (u *Unpackerr) validateConfig() {
	if u.DeleteDelay.Duration < minimumDeleteDelay {
		u.DeleteDelay.Duration = minimumDeleteDelay
		u.Debug("Minimum Delete Delay: %v", minimumDeleteDelay.String())
	}

	if u.Parallel == 0 {
		u.Parallel++
	}

	if u.Buffer == 0 {
		u.Buffer = defaultQueueSize
	} else if u.Buffer < minimumQueueSize {
		u.Buffer = minimumQueueSize
	}

	if u.Interval.Duration < minimumInterval {
		u.Interval.Duration = minimumInterval
		u.Debug("Minimum Interval: %v", minimumInterval.String())
	}

	for i := range u.Radarr {
		if u.Radarr[i].Timeout.Duration == 0 {
			u.Radarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Radarr[i].Path == "" {
			u.Radarr[i].Path = defaultSavePath
		}

		if u.Radarr[i].Protocols == "" {
			u.Radarr[i].Protocols = defaultProtocol
		}
	}

	for i := range u.Sonarr {
		if u.Sonarr[i].Timeout.Duration == 0 {
			u.Sonarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Sonarr[i].Path == "" {
			u.Sonarr[i].Path = defaultSavePath
		}

		if u.Sonarr[i].Protocols == "" {
			u.Sonarr[i].Protocols = defaultProtocol
		}
	}

	for i := range u.Lidarr {
		if u.Lidarr[i].Timeout.Duration == 0 {
			u.Lidarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Lidarr[i].Path == "" {
			u.Lidarr[i].Path = defaultSavePath
		}

		if u.Lidarr[i].Protocols == "" {
			u.Lidarr[i].Protocols = defaultProtocol
		}
	}

	for i := range u.Readarr {
		if u.Readarr[i].Timeout.Duration == 0 {
			u.Readarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Readarr[i].Path == "" {
			u.Readarr[i].Path = defaultSavePath
		}

		if u.Readarr[i].Protocols == "" {
			u.Readarr[i].Protocols = defaultProtocol
		}
	}
}

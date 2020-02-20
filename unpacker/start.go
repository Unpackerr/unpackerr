package unpacker

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golift.io/cnfg"
	"golift.io/cnfg/cnfgfile"
	"golift.io/xtractr"

	"github.com/prometheus/common/version"
	flg "github.com/spf13/pflag"
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
		return fmt.Errorf("Config File: %v", err)
	}

	if _, err := cnfg.UnmarshalENV(u.Config, "UN"); err != nil {
		return fmt.Errorf("Environment Variables: %v", err)
	}

	if err := u.setupLogging(); err != nil {
		return fmt.Errorf("Log File Config: %v", err)
	}

	u.Logf("Unpackerr v%s Starting! (PID: %v) %v", version.Version, os.Getpid(), time.Now())

	u.validateConfig()
	u.printStartupInfo()

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
		u.Debug("Minimum Delete Delay: %v", minimumDeleteDelay.String())
	}

	if u.Parallel == 0 {
		u.Parallel++
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

func (u *Unpackerr) printStartupInfo() {
	const oneItem = 1

	u.Log("==> Startup Settings <==")

	if c := len(u.Sonarr); c == oneItem {
		u.Logf(" => Sonarr Config: 1 server: %s @ %s (apikey: %v, timeout: %v)",
			u.Sonarr[0].URL, u.Sonarr[0].Path, u.Sonarr[0].APIKey != "", u.Sonarr[0].Timeout)
	} else {
		u.Log(" => Sonarr Config:", c, "servers")

		for _, f := range u.Sonarr {
			u.Logf(" =>    Server: %s @ %s (apikey: %v, timeout: %v)",
				f.URL, f.Path, f.APIKey != "", f.Timeout)
		}
	}

	if c := len(u.Radarr); c == oneItem {
		u.Logf(" => Radarr Config: 1 server: %s @ %s (apikey: %v, timeout: %v)",
			u.Radarr[0].URL, u.Radarr[0].Path, u.Radarr[0].APIKey != "", u.Radarr[0].Timeout)
	} else {
		u.Log(" => Radarr Config:", c, "servers")

		for _, f := range u.Radarr {
			u.Logf(" =>    Server: %s @ %s (apikey: %v, timeout: %v)",
				f.URL, f.Path, f.APIKey != "", f.Timeout)
		}
	}

	if c := len(u.Lidarr); c == oneItem {
		u.Logf(" => Lidarr Config: 1 server: %s (apikey: %v, timeout: %v)",
			u.Lidarr[0].URL, u.Lidarr[0].APIKey != "", u.Lidarr[0].Timeout)
	} else {
		u.Log(" => Lidarr Config:", c, "servers")

		for _, f := range u.Lidarr {
			u.Logf(" =>    Server: %s (apikey: %v, timeout: %v)", f.URL, f.APIKey != "", f.Timeout)
		}
	}

	if c := len(u.Folders); c == oneItem {
		u.Logf(" => Folder Config: 1 path: %s (delete after:%v, delete orig:%v, move back:%v)",
			u.Folders[0].Path, u.Folders[0].DeleteAfter, u.Folders[0].DeleteOrig, u.Folders[0].MoveBack)
	} else {
		u.Log(" => Folder Config:", c, "paths")

		for _, f := range u.Folders {
			u.Logf(" =>    Path: %s (delete after:%v, delete orig:%v, move back:%v)",
				f.Path, f.DeleteAfter, f.DeleteOrig, f.MoveBack)
		}
	}

	u.Log(" => Parallel:", u.Config.Parallel)
	u.Log(" => Interval:", u.Config.Interval.Duration)
	u.Log(" => Delete Delay:", u.Config.DeleteDelay.Duration)
	u.Log(" => Start Delay:", u.Config.StartDelay.Duration)
	u.Log(" => Retry Delay:", u.Config.RetryDelay.Duration)
	u.Log(" => Debug / Quiet:", u.Config.Debug, "/", u.Config.Quiet)
	u.Log(" => Log File:", u.Config.LogFile)
}

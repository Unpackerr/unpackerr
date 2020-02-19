package unpacker

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

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
	u.printStartupInfo()

	if u.Config.Debug {
		log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
	}

	u.Xtractr = xtractr.NewQueue(&xtractr.Config{
		Debug:    u.Config.Debug,
		Parallel: int(u.Parallel),
		Suffix:   suffix,
		Logger:   log.New(os.Stdout, "", log.Flags()),
	})

	u.PollFolders() // this initializes channel(s) used in u.Run()

	go u.Run()
	signal.Notify(u.sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	log.Println("=====> Exiting! Caught Signal:", <-u.sigChan)

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
		u.DeLogf("Minimum Delete Delay: %v", minimumDeleteDelay.String())
	}

	if u.Parallel == 0 {
		u.Parallel++
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

func (u *Unpackerr) printStartupInfo() {
	const oneItem = 1

	log.Println("==> Startup Settings <==")

	if c := len(u.Sonarr); c == oneItem {
		log.Printf(" => Sonarr Config: 1 server: %s @ %s (apikey: %v, timeout: %v)",
			u.Sonarr[0].URL, u.Sonarr[0].Path, u.Sonarr[0].APIKey != "", u.Sonarr[0].Timeout)
	} else {
		log.Println(" => Sonarr Config:", c, "servers")

		for _, f := range u.Sonarr {
			log.Printf(" =>    Server: %s @ %s (apikey: %v, timeout: %v)",
				f.URL, f.Path, f.APIKey != "", f.Timeout)
		}
	}

	if c := len(u.Radarr); c == oneItem {
		log.Printf(" => Radarr Config: 1 server: %s @ %s (apikey: %v, timeout: %v)",
			u.Radarr[0].URL, u.Radarr[0].Path, u.Radarr[0].APIKey != "", u.Radarr[0].Timeout)
	} else {
		log.Println(" => Radarr Config:", c, "servers")

		for _, f := range u.Radarr {
			log.Printf(" =>    Server: %s @ %s (apikey: %v, timeout: %v)",
				f.URL, f.Path, f.APIKey != "", f.Timeout)
		}
	}

	if c := len(u.Lidarr); c == oneItem {
		log.Printf(" => Lidarr Config: 1 server: %s (apikey: %v, timeout: %v)",
			u.Lidarr[0].URL, u.Lidarr[0].APIKey != "", u.Lidarr[0].Timeout)
	} else {
		log.Println(" => Lidarr Config:", c, "servers")

		for _, f := range u.Lidarr {
			log.Printf(" =>    Server: %s (apikey: %v, timeout: %v)", f.URL, f.APIKey != "", f.Timeout)
		}
	}

	if c := len(u.Folders); c == oneItem {
		log.Printf(" => Folder Config: 1 path: %s (delete after:%v, delete orig:%v, move back:%v)",
			u.Folders[0].Path, u.Folders[0].DeleteAfter, u.Folders[0].DeleteOrig, u.Folders[0].MoveBack)
	} else {
		log.Println(" => Folder Config:", c, "paths")

		for _, f := range u.Folders {
			log.Printf(" =>    Path: %s (delete after:%v, delete orig:%v, move back:%v)",
				f.Path, f.DeleteAfter, f.DeleteOrig, f.MoveBack)
		}
	}

	log.Println(" => Parallel:", u.Config.Parallel)
	log.Println(" => Interval:", u.Config.Interval.Duration)
	log.Println(" => Delete Delay:", u.Config.DeleteDelay.Duration)
	log.Println(" => Start Delay:", u.Config.StartDelay.Duration)
	log.Println(" => Retry Delay:", u.Config.RetryDelay.Duration)
	log.Println(" => Debug Logs:", u.Config.Debug)
}

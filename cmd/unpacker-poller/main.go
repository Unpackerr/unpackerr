package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/davidnewhall/unpacker-poller/deluge"
	"github.com/davidnewhall/unpacker-poller/starr"

	"github.com/naoina/toml"
	flg "github.com/ogier/pflag"
	"github.com/pkg/errors"
)

const (
	defaultConfFile = "/usr/local/etc/unpacker-poller/up.conf"
	minimumInterval = 1 * time.Minute
	defaultTimeout  = 10 * time.Second
)

var (
	// Version of the aplication.
	Version = "0.1.2"
	// Debug turns on the noise.
	Debug = false
	// ConfigFile is the file we get configuration from.
	ConfigFile = ""
	// StopChan is how we exit. Can be used in tests.
	StopChan = make(chan os.Signal, 1)
)

func main() {
	ParseFlags()
	log.Printf("Unpacker Poller Starting! (PID: %v)", os.Getpid())
	config, err := GetConfig(ConfigFile)
	if err != nil {
		log.Fatalln("ERROR (config):", err)
	}
	d, err := deluge.New(*config.Deluge)
	if err != nil {
		log.Fatalln("ERROR (deluge):", err)
	}
	go StartUp(d, config)
	signal.Notify(StopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	log.Println("\nExiting! Caught Signal:", <-StopChan)
}

// ParseFlags turns CLI args into usable data.
func ParseFlags() {
	flg.Usage = func() {
		fmt.Println("Usage: unpacker-poller [--config=filepath] [--debug] [--version]")
		flg.PrintDefaults()
	}
	flg.StringVarP(&ConfigFile, "config", "c", defaultConfFile, "Poller Config File (TOML Format)")
	flg.BoolVarP(&Debug, "debug", "D", false, "Turn on the Spam (default false)")
	version := flg.BoolP("version", "v", false, "Print the version and exit.")
	flg.Parse()
	if *version {
		fmt.Println("unpacker-poller version:", Version)
		os.Exit(0) // don't run anything else.
	}
	if log.SetFlags(log.LstdFlags); Debug {
		log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
	}
}

// GetConfig parses and returns our configuration data.
func GetConfig(configFile string) (Config, error) {
	// Preload our defaults.
	config := Config{}
	if Debug {
		log.Println("Reading Config File:", configFile)
	}
	if buf, err := ioutil.ReadFile(configFile); err != nil {
		return config, err
		// This is where the defaults in the config variable are overwritten.
	} else if err := toml.Unmarshal(buf, &config); err != nil {
		return config, errors.Wrap(err, "invalid config")
	}
	return ValidateConfig(config)
}

// ValidateConfig makes sure config values are ok.
func ValidateConfig(config Config) (Config, error) {
	// Fix up intervals.
	if config.Timeout.Value() == 0 {
		log.Println("Setting Default Timeout:", defaultTimeout.String())
		config.Timeout.Set(defaultTimeout)
	}
	if config.Deluge.Timeout.Value() == 0 {
		config.Deluge.Timeout.Set(config.Timeout.Value())
	}
	if config.Radarr.Timeout.Value() == 0 {
		config.Radarr.Timeout.Set(config.Timeout.Value())
	}
	if config.Sonarr.Timeout.Value() == 0 {
		config.Sonarr.Timeout.Set(config.Timeout.Value())
	}

	if config.Interval.Value() < minimumInterval {
		log.Println("Setting Minimum Interval:", minimumInterval.String())
		config.Interval.Set(minimumInterval)
	}
	if config.Deluge.Interval.Value() == 0 {
		config.Deluge.Interval.Set(config.Interval.Value())
	}
	if config.Radarr.Interval.Value() == 0 {
		config.Radarr.Interval.Set(config.Interval.Value())
	}
	if config.Sonarr.Interval.Value() == 0 {
		config.Sonarr.Interval.Set(config.Interval.Value())
	}
	return config, nil
}

// StartUp all the go routines.
func StartUp(d *deluge.Deluge, config Config) {
	var r RunningData
	r.History = make(map[string]Extracts)
	go r.PollDeluge(d)
	if config.Sonarr.APIKey != "" {
		time.Sleep(time.Second * 5) // spread out the http checks a bit.
		go r.PollSonarr(config.Sonarr)
	}
	if config.Radarr.APIKey != "" {
		time.Sleep(time.Second * 5)
		go r.PollRadarr(config.Radarr)
	}
	go r.PollChange()
}

// PollDeluge at an interval and save the transfer list to r.Deluge
func (r *RunningData) PollDeluge(d *deluge.Deluge) {
	log.Printf("Deluge Poller Starting: %v (interval: %v)", d.URL, d.Interval.String())
	ticker := time.NewTicker(d.Interval).C
	for range ticker {
		var err error
		r.delS.Lock()
		if r.Deluge, err = d.GetXfers(); err != nil {
			log.Println("Deluge Error:", err)
		} else {
			log.Println("Deluge Updated:", len(r.Deluge), "Transfers")
		}
		r.delS.Unlock()
	}
}

// PollSonarr saves the Sonarr Queue to r.SonarrQ
func (r *RunningData) PollSonarr(s *starr.Config) {
	log.Printf("Sonarr Poller Starting: %v (interval: %v)", s.URL, s.Interval.String())
	ticker := time.NewTicker(s.Interval.Value()).C
	for range ticker {
		var err error
		r.sonS.Lock()
		if r.SonarrQ, err = starr.SonarrQueue(*s); err != nil {
			log.Println("Sonarr Error:", err)
		} else {
			log.Println("Sonarr Updated:", len(r.SonarrQ), "Items Queued")
		}
		r.sonS.Unlock()
	}
}

// PollRadarr saves the Radarr Queue to r.RadarrQ
func (r *RunningData) PollRadarr(s *starr.Config) {
	log.Printf("Radarr Poller Starting: %v (interval: %v)", s.URL, s.Interval.String())
	ticker := time.NewTicker(s.Interval.Value()).C
	for range ticker {
		var err error
		r.radS.Lock()
		if r.RadarrQ, err = starr.RadarrQueue(*s); err != nil {
			log.Println("Radarr Error:", err)
		} else {
			log.Println("Radarr Updated:", len(r.RadarrQ), "Items Queued")
		}
		r.radS.Unlock()
	}
}

// PollChange runs other tasks.
// Those tasks: a) look for things to extract, b) look for things to delete.
func (r *RunningData) PollChange() {
	// Don't start this for 2 whole minutes.
	time.Sleep(time.Minute)
	log.Println("Starting Cleanup Routine (interval: 1m0s)")
	// This runs more often because of the cleanup tasks.
	// It doesn't poll external data, unless it finds something to extract.
	ticker := time.NewTicker(time.Minute).C
	for range ticker {
		if r.Deluge == nil {
			// No data.
			continue
		}
		r.CheckExtractDone()
		if r.SonarrQ != nil {
			r.CheckSonarrQueue()
		}
		if r.RadarrQ != nil {
			r.CheckRadarrQueue()
		}
	}
}

// CheckExtractDone checks if an extracted item has been imported so it may be deleted.
func (r *RunningData) CheckExtractDone() {
	for name, data := range r.GetHistory() {
		if data.Status < DELETED {
			log.Printf("Extract Statuses: %v (status: %v, elapsed: %v)", name, data.Status.String(), time.Now().Sub(data.Updated).Round(time.Second))
		}
		switch {
		case data.Status != EXTRACTED:
			// Only check for and process items that have finished extraction.
			continue
		case data.App == "Sonarr":
			if sq := r.getSonarQitem(name); sq.Status == "" {
				// TODO: delete_delay -> r.UpdateStatus(name, IMPORTED, nil) -> timer
				log.Println("Sonarr Imported:", name)
				r.UpdateStatus(name, DELETING, nil)
				go r.deleteFiles(name, data.FileList)
			} else if Debug {
				log.Println("Sonarr Item Waiting For Import:", name, "->", sq.Status)
			}
		case data.App == "Radarr":
			if rq := r.getRadarQitem(name); rq.Status == "" {
				log.Println("Radarr Imported:", name)
				r.UpdateStatus(name, DELETING, nil)
				go r.deleteFiles(name, data.FileList)
			} else if Debug {
				log.Println("Radarr Item Waiting For Import:", name, "->", rq.Status)
			}
		}
	}
}

// CheckSonarrQueue passes completed Sonarr-queued downloads to the HandleCompleted method.
func (r *RunningData) CheckSonarrQueue() {
	r.sonS.RLock()
	defer r.sonS.RUnlock()
	for _, q := range r.SonarrQ {
		if q.Status == "Completed" {
			go r.HandleCompleted(q.Title, "Sonarr")
		} else if Debug {
			log.Printf("Sonarr: %v (%d%%): %v (Ep: %v)", q.Status, int(100-(q.Sizeleft/q.Size*100)), q.Title, q.Episode.Title)
		}
	}
}

// CheckRadarrQueue passes completed Radarr-queued downloads to the HandleCompleted method.
func (r *RunningData) CheckRadarrQueue() {
	r.radS.RLock()
	defer r.radS.RUnlock()
	for _, q := range r.RadarrQ {
		if q.Status == "Completed" {
			go r.HandleCompleted(q.Title, "Radarr")
		} else if Debug {
			log.Printf("Radarr: %v (%d%%): %v", q.Status, int(100-(q.Sizeleft/q.Size*100)), q.Title)
		}
	}
}

// HandleCompleted checks if a completed sonarr or radarr item needs to be extracted.
func (r *RunningData) HandleCompleted(name, app string) {
	d := r.getXfer(name)
	if d.Name == "" {
		if Debug {
			log.Printf("%v: Transfer not found in Deluge: %v", app, name)
		}
		return
	}
	path := filepath.Join(d.SavePath, d.Name)
	file := findRarFile(path)
	if d.IsFinished && r.GetStatus(name).Status == UNKNOWN {
		if file != "" {
			log.Printf("%v: Found extractable item in Deluge: %v -rar-file-> %v", app, name, file)
			r.CreateStatus(name, path, file, app, QUEUED)
			r.extractFile(name, path, file)
		} else if Debug {
			log.Printf("%v: Completed Item still in Queue: %v", app, name)
		}
	}
}

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
)

var (
	// Version of the aplication.
	Version = "0.1.1"
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
		deluge.Debug = true
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
	if config.Interval.value < minimumInterval {
		log.Println("Setting Minimum Interval:", minimumInterval.String())
		config.Interval.value = minimumInterval
	}
	return config, nil
}

// StartUp all the go routines.
func StartUp(d *deluge.Deluge, config Config) {
	var r RunningData
	r.History = make(map[string]Extracts)
	log.Printf("Deluge Poller Starting: %v (interval: %v)", config.Deluge.URL, config.Interval.value.String())
	go r.PollDeluge(d, config.Interval.value)
	if config.Sonarr.APIKey != "" {
		time.Sleep(time.Second * 5) // spread out the http checks a bit.
		log.Printf("Sonarr Poller Starting: %v (interval: %v)", config.Sonarr.URL, config.Interval.value.String())
		go r.PollSonarr(config.Sonarr, config.Interval.value)
	}
	if config.Radarr.APIKey != "" {
		time.Sleep(time.Second * 5)
		log.Printf("Radarr Poller Starting: %v (interval: %v)", config.Sonarr.URL, config.Interval.value.String())
		go r.PollRadarr(config.Radarr, config.Interval.value)
	}
	go r.PollChange()
}

// PollDeluge at an interval and save the transfer list to r.Deluge
func (r *RunningData) PollDeluge(d *deluge.Deluge, interval time.Duration) {
	ticker := time.NewTicker(interval).C
	for range ticker {
		var err error
		r.delS.Lock()
		if r.Deluge, err = d.GetXfers(); err != nil {
			log.Println("Deluge Error:", err)
		} else {
			log.Println("Deluge:", len(r.Deluge), "Transfers")
		}
		r.delS.Unlock()
	}
}

// PollSonarr saves the Sonarr Queue to r.SonarrQ
func (r *RunningData) PollSonarr(s *starr.Config, interval time.Duration) {
	ticker := time.NewTicker(interval).C
	for range ticker {
		var err error
		r.sonS.Lock()
		if r.SonarrQ, err = starr.SonarrQueue(*s); err != nil {
			log.Println("Sonarr Error:", err)
		} else {
			log.Println("Sonarr:", len(r.SonarrQ), "Items Queued")
		}
		r.sonS.Unlock()
	}
}

// PollRadarr saves the Radarr Queue to r.RadarrQ
func (r *RunningData) PollRadarr(s *starr.Config, interval time.Duration) {
	ticker := time.NewTicker(interval).C
	for range ticker {
		var err error
		r.radS.Lock()
		if r.RadarrQ, err = starr.RadarrQueue(*s); err != nil {
			log.Println("Radarr Error:", err)
		} else {
			log.Println("Radarr:", len(r.RadarrQ), "Items Queued")
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
			log.Printf("Extraction: %v (status: %v, duration: %v)", name, data.Status.String(), time.Now().Sub(data.Updated).Round(time.Second))
		}
		switch {
		case data.Status != EXTRACTED:
			// Only check for and process items that have finished extraction.
			continue
		case data.App == "Sonarr":
			if sq := r.getSonarQitem(name); sq.Status == "" {
				// TODO: delete_delay -> r.UpdateStatus(name, IMPORTED, nil) -> timer
				r.UpdateStatus(name, DELETING, nil)
				log.Println("Sonarr Extracted Item Imported:", name)
				go r.deleteFiles(name, data.FileList)
			} else if Debug {
				log.Println("Sonarr Item Waiting For Import:", name, "->", sq.Status)
			}
		case data.App == "Radarr":
			if rq := r.getRadarQitem(name); rq.Status == "" {
				r.UpdateStatus(name, DELETING, nil)
				log.Println("Radarr Extracted Item Imported:", name)
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
		if Debug {
			log.Printf("Sonarr: %v (%d%%): %v (Ep: %v)", q.Status, int(100-(q.Sizeleft/q.Size*100)), q.Title, q.Episode.Title)
		}
		if q.Status != "Completed" {
			// Only process Completed items.
			continue
		}
		go r.HandleCompleted(q.Title, "Sonarr")
	}
}

// CheckRadarrQueue passes completed Radarr-queued downloads to the HandleCompleted method.
func (r *RunningData) CheckRadarrQueue() {
	r.radS.RLock()
	defer r.radS.RUnlock()
	for _, q := range r.RadarrQ {
		if Debug {
			log.Printf("Radarr: %v (%d%%): %v", q.Status, int(100-(q.Sizeleft/q.Size*100)), q.Title)
		}
		if q.Status != "Completed" {
			continue
		}
		go r.HandleCompleted(q.Title, "Radarr")
	}
}

// HandleCompleted checks if a completed sonarr or radarr item needs to be extracted.
func (r *RunningData) HandleCompleted(name, app string) {
	d := r.getXfer(name)
	if d.Name == "" {
		log.Printf("%v: Unusual, transfer not found in Deluge: %v", app, name)
		return
	}
	path := filepath.Join(d.SavePath, d.Name)
	file := findRarFile(path)
	if file != "" && d.IsFinished && r.GetStatus(name).Status == UNKNOWN {
		log.Printf("%v: Found extractable item in Deluge: %v -rar-file-> %v", app, name, file)
		r.CreateStatus(name, path, file, app, QUEUED)
		r.extractFile(name, path, file)
	}
}

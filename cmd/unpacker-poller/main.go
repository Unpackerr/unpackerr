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

const defaultConfFile = "./up.conf"

var (
	// Version of the aplication.
	Version = "0.1.0"
	// Debug turns on the noise.
	Debug = false
	// ConfigFile is the file we get configuration from.
	ConfigFile = ""
	// StopChan is how we exit.
	StopChan = make(chan os.Signal, 1)
)

func main() {
	ParseFlags()
	log.Println("unpacker-poller starting!")
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
	if log.SetFlags(0); Debug {
		log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
		deluge.Debug = true
	}
}

// GetConfig parses and returns our configuration data.
func GetConfig(configFile string) (Config, error) {
	// Preload our defaults.
	config := Config{}
	if buf, err := ioutil.ReadFile(configFile); err != nil {
		return config, err
		// This is where the defaults in the config variable are overwritten.
	} else if err := toml.Unmarshal(buf, &config); err != nil {
		return config, errors.Wrap(err, "invalid config")
	}
	return config, nil
}

// StartUp all the go routines.
func StartUp(d *deluge.Deluge, config Config) {
	var r runningData
	r.History = make(map[string]extracts)
	log.Println("Starting Deluge Poller:", config.Deluge.URL)
	go r.PollDeluge(d, config.Interval.value)
	if config.Sonarr.APIKey != "" {
		time.Sleep(time.Second)
		log.Println("Starting Sonarr Poller:", config.Sonarr.URL)
		go r.PollSonarr(config.Sonarr, config.Interval.value)
	}
	if config.Radarr.APIKey != "" {
		time.Sleep(time.Second)
		log.Println("Starting Radarr Poller:", config.Sonarr.URL)
		go r.PollRadarr(config.Radarr, config.Interval.value)
	}
	go r.PollChange()
}

// PollDeluge at an interval and look for things to extract.
func (r *runningData) PollDeluge(d *deluge.Deluge, interval time.Duration) {
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

// PollSonarr and save the Queue to r.SonarrQ
func (r *runningData) PollSonarr(s *starr.Config, interval time.Duration) {
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

// PollRadarr and save the Queue to r.RadarrQ
func (r *runningData) PollRadarr(s *starr.Config, interval time.Duration) {
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

// PollChange kicks off other tasks.
// Those tasks: a) look for things to extract, b) look for things to delete.
func (r *runningData) PollChange() {
	// Don't start this for 2 whole minutes.
	time.Sleep(60 * time.Second)
	log.Println("Starting Completion Handler Routine")
	ticker := time.NewTicker(60 * time.Second).C
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
func (r *runningData) CheckExtractDone() {
	for name, data := range r.GetHistory() {
		if data.Status != "deleted" {
			log.Println("History:", name, "Status:", data.Status, "In This State:", time.Now().Sub(data.Time).Round(time.Second))
		}
		// Only check for and process items that have finished extraction.
		if data.Status != "extracted" {
			continue
		}
		switch data.App {
		case "Sonarr":
			if sq := r.getSonarQitem(name); sq.Status == "" {
				go r.deleteFiles(name, data.FileList)
			} else {
				log.Println("Sonarr Status:", name, "->", sq.Status)
			}
		case "Radarr":
			if rq := r.getRadarQitem(name); rq.Status == "" {
				go r.deleteFiles(name, data.FileList)
			} else {
				log.Println("Radarr Status:", name, "->", rq.Status)
			}
		}
	}
}

// CheckSonarrQueue passes completed Sonarr-queued downloads to the HandleCompleted method.
func (r *runningData) CheckSonarrQueue() {
	r.sonS.RLock()
	defer r.sonS.RUnlock()
	for _, q := range r.SonarrQ {
		// Only process Completed items.
		if q.Status != "Completed" {
			log.Printf("Sonarr: (status: %v) Unfinished (%d%%): %v (Ep: %v)", q.Status, int(100-(q.Sizeleft/q.Size*100)), q.Title, q.Episode.Title)
			continue
		}
		go r.HandleCompleted(q.Title, "Sonarr")
	}
}

// CheckSonarrQueue passes completed Radarr-queued downloads to the HandleCompleted method.
func (r *runningData) CheckRadarrQueue() {
	r.radS.RLock()
	defer r.radS.RUnlock()
	for _, q := range r.RadarrQ {
		if q.Status != "Completed" {
			log.Printf("Radarr: (status: %v) Unfinished (%d%%): %v", q.Status, int(100-(q.Sizeleft/q.Size*100)), q.Title)
			continue
		}
		go r.HandleCompleted(q.Title, "Radarr")
	}
}

// HandleCompleted checks if a completed sonarr or radarr item needs to be extracted.
func (r *runningData) HandleCompleted(name, app string) {
	d := r.getXfer(name)
	if d.Name == "" {
		log.Printf("%v: Transfer not found in Deluge: %v", app, name)
		return
	}
	path := filepath.Join(d.SavePath, d.Name)
	file := findRarFile(path)
	if file != "" && d.IsFinished && r.GetStatus(name).Status == "" {
		log.Printf("%v: Found completed item in Deluge: %v -rar-file-> %v", app, path, file)
		r.CreateStatus(name, path, file, "queued", app)
		r.extractFile(name, path, file)
	}
}

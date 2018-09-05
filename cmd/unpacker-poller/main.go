package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/davidnewhall/unpacker-poller/deluge"

	"github.com/naoina/toml"
	flg "github.com/ogier/pflag"
	"github.com/pkg/errors"
)

const (
	defaultConfFile    = "/usr/local/etc/unpacker-poller/up.conf"
	minimumInterval    = 1 * time.Minute
	minimumDeleteDelay = 1 * time.Minute
	defaultTimeout     = 10 * time.Second
)

var (
	// Version of the aplication.
	Version = "0.1.4"
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
	DeLogf("Reading Config File: %v", configFile)
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
	if config.DeleteDelay.Value() < minimumDeleteDelay {
		DeLogf("Setting Minimum Delete Delay: %v", minimumDeleteDelay.String())
		config.DeleteDelay.Set(minimumDeleteDelay)
	}
	if config.ConcurrentExtracts < 1 {
		config.ConcurrentExtracts = 1
	} else if config.ConcurrentExtracts > 10 {
		config.ConcurrentExtracts = 10
	}
	DeLogf("Maximum Concurrent Extractions: %d", config.ConcurrentExtracts)
	// Fix up intervals.
	if config.Timeout.Value() == 0 {
		DeLogf("Setting Default Timeout: %v", defaultTimeout.String())
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
		DeLogf("Setting Minimum Interval: %v", minimumInterval.String())
		config.Interval.Set(minimumInterval)
	}
	return config, nil
}

// StartUp all the go routines.
func StartUp(d *deluge.Deluge, config Config) {
	r := RunningData{
		DeleteDelay: config.DeleteDelay.Value(),
		maxExtracts: config.ConcurrentExtracts,
		History:     make(map[string]Extracts),
	}
	go r.PollChange() // This has its own ticker tha runs every minute.
	go func() {
		// Run all pollers once at startup.
		r.pollAllApps(config, d)
		ticker := time.NewTicker(config.Interval.Value()).C
		for range ticker {
			r.pollAllApps(config, d)
		}
	}()
}

// Poll Deluge, Sonarr and Radarr. All at the same time.
func (r *RunningData) pollAllApps(config Config, d *deluge.Deluge) {
	go func() {
		if r.PollDeluge(d) != nil {
			newDeluge, err := deluge.New(*config.Deluge)
			if err != nil {
				log.Println("Deluge Authentication Error:", err)
				// When auth fails > 1 time while running, just exit. Only exit if things are not pending.
				// if r.eCount().extracting == 0 && r.eCount().extracted == 0 && r.eCount().imported == 0 && r.eCount().queued == 0 {
				// 	os.Exit(2)
				// }
			}
			*d = *newDeluge
		}
	}()
	if config.Sonarr.APIKey != "" {
		go r.PollSonarr(config.Sonarr)
	}
	if config.Radarr.APIKey != "" {
		go r.PollRadarr(config.Radarr)
	}
}

// DeLogf writes Debug log lines.
func DeLogf(msg string, v ...interface{}) {
	if Debug {
		log.Printf("[DEBUG] "+msg, v...)
	}
}

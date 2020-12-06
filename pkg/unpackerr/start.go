package unpackerr

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	flag "github.com/spf13/pflag"
	"golift.io/cnfg"
	"golift.io/cnfg/cnfgfile"
	"golift.io/version"
	"golift.io/xtractr"
)

const (
	defaultFileMode    = 0644
	defaultDirMode     = 0755
	defaultTimeout     = 10 * time.Second
	minimumInterval    = 15 * time.Second
	defaultRetryDelay  = 5 * time.Minute
	defaultStartDelay  = time.Minute
	minimumDeleteDelay = time.Second
	suffix             = "_unpackerred" // suffix for unpacked folders.
	mebiByte           = 1024 * 1024    // Used to turn bytes in MiB.
	updateChanBuf      = 100            // Size of xtractr callback update channels.
	defaultFolderBuf   = 20000          // Channel queue size for file system events.
	minimumFolderBuf   = 1000           // Minimum size of the folder event buffer.
	defaultLogFileMb   = 10
	defaultLogFiles    = 10
	helpLink           = "GoLift Discord: https://golift.io/discord" // prints on start and on exit.
)

// Unpackerr stores all the running data.
type Unpackerr struct {
	*Flags
	*Config
	*History
	*xtractr.Xtractr
	folders *Folders
	sigChan chan os.Signal
	updates chan *xtractr.Response
	*Logger
}

// Logger provides a struct we can pass into other packages.
type Logger struct {
	debug  bool
	Logger *log.Logger
}

// Flags are our CLI input flags.
type Flags struct {
	verReq     bool
	ConfigFile string
	EnvPrefix  string
	webhook    uint
}

// History holds the history of extracted items.
type History struct {
	Finished  uint
	Restarted uint
	Map       map[string]*Extract
}

// New returns an UnpackerPoller struct full of defaults.
// An empty struct will surely cause you pain, so use this!
func New() *Unpackerr {
	return &Unpackerr{
		Flags:   &Flags{ConfigFile: defaultConfFile, EnvPrefix: "UN"},
		sigChan: make(chan os.Signal),
		History: &History{Map: make(map[string]*Extract)},
		updates: make(chan *xtractr.Response, updateChanBuf),
		Config: &Config{
			LogFiles:    defaultLogFiles,
			Timeout:     cnfg.Duration{Duration: defaultTimeout},
			Interval:    cnfg.Duration{Duration: minimumInterval},
			RetryDelay:  cnfg.Duration{Duration: defaultRetryDelay},
			StartDelay:  cnfg.Duration{Duration: defaultStartDelay},
			DeleteDelay: cnfg.Duration{Duration: minimumDeleteDelay},
		},
		Logger: &Logger{Logger: log.New(ioutil.Discard, "", 0)},
	}
}

// Start runs the app.
func Start() (err error) {
	log.SetFlags(log.LstdFlags) // in case we throw an error for main.go before logging is setup.

	u := New().ParseFlags() // Grab CLI args (like config file location).
	if u.Flags.verReq {
		fmt.Println(version.Print("unpackerr"))

		return nil // don't run anything else.
	}

	if err := cnfgfile.Unmarshal(u.Config, u.ConfigFile); err != nil {
		return fmt.Errorf("config file: %w", err)
	}

	if _, err := cnfg.UnmarshalENV(u.Config, u.Flags.EnvPrefix); err != nil {
		return fmt.Errorf("environment variables: %w", err)
	}

	fm, dm := u.validateConfig()
	// Do not do any logging before this.
	// ie. No running of u.Debugf or u.Print* before running validateConfig()

	if u.Flags.webhook > 0 {
		return u.sampleWebhook(ExtractStatus(u.Flags.webhook))
	}

	u.logStartupInfo()

	u.Xtractr = xtractr.NewQueue(&xtractr.Config{
		Parallel: int(u.Parallel),
		Suffix:   suffix,
		Logger:   u.Logger,
		FileMode: os.FileMode(fm),
		DirMode:  os.FileMode(dm),
	})

	go u.Run()
	signal.Notify(u.sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	u.Printf("[unpackerr] Need help? %s\n=====> Exiting! Caught Signal: %v", helpLink, <-u.sigChan)

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
	flag.UintVarP(&u.Flags.webhook, "webhook", "w", 0, "Send test webhook. Valid values: 1,2,3,4,5,6,7,8")
	flag.BoolVarP(&u.Flags.verReq, "version", "v", false, "Print the version and exit.")
	flag.Parse()

	return u // so you can chain into ParseConfig.
}

// Run starts the loop that does the work.
func (u *Unpackerr) Run() {
	var (
		poller  = time.NewTicker(u.Interval.Duration) // poll apps at configured interval.
		cleaner = time.NewTicker(minimumInterval)     // clean at the minimum interval.
		logger  = time.NewTicker(time.Minute)         // log queue states every minute.
	)

	u.PollFolders()      // This initializes channel(s) used below.
	u.processAppQueues() // Get in-app queues on startup.

	// one go routine to rule them all.
	for {
		select {
		case <-cleaner.C:
			// Check for state changes and act on them.
			u.checkExtractDone()
			u.checkFolderStats()
		case <-poller.C:
			// polling interval. pull API data from all apps.
			u.processAppQueues()
			// check if things got imported and now need to be deleted.
			u.checkImportsDone()
		case resp := <-u.updates:
			// xtractr callback for arr app download extraction.
			u.handleXtractrCallback(resp)
		case resp := <-u.folders.Updates:
			// xtractr callback for a watched folder extraction.
			u.folderXtractrCallback(resp)
		case event := <-u.folders.Events:
			// file system event for watched folder.
			u.folders.processEvent(event)
		case <-logger.C:
			// Log/print current queue counts once in a while.
			u.logCurrentQueue()
		}
	}
}

// validateConfig makes sure config file values are ok. Returns file and dir modes.
func (u *Unpackerr) validateConfig() (uint64, uint64) {
	if u.DeleteDelay.Duration < minimumDeleteDelay {
		u.DeleteDelay.Duration = minimumDeleteDelay
	}

	fm, err := strconv.ParseUint(u.FileMode, 8, 32)
	if err != nil || u.FileMode == "" {
		fm = defaultFileMode
		u.FileMode = strconv.FormatUint(fm, 32)
	}

	dm, err := strconv.ParseUint(u.DirMode, 8, 32)
	if err != nil || u.DirMode == "" {
		dm = defaultDirMode
		u.DirMode = strconv.FormatUint(dm, 32)
	}

	if u.Parallel == 0 {
		u.Parallel++
	}

	if u.Buffer == 0 {
		u.Buffer = defaultFolderBuf
	} else if u.Buffer < minimumFolderBuf {
		u.Buffer = minimumFolderBuf
	}

	if u.Interval.Duration < minimumInterval {
		u.Interval.Duration = minimumInterval
	}

	if u.Config.Debug && u.LogFiles == defaultLogFiles {
		u.LogFiles *= 2 // Double default if debug is turned on.
	}

	if u.LogFileMb == 0 {
		if u.LogFileMb = defaultLogFileMb; u.Config.Debug {
			u.LogFileMb *= 2 // Double default if debug is turned on.
		}
	}

	// We cannot log anything until setupLogging() runs.
	// We cannot run setupLogging until we read the above config.
	u.setupLogging()
	u.Printf("Unpackerr v%s Starting! (PID: %v) %v", version.Version, os.Getpid(), version.Started)
	u.validateApps()

	return fm, dm
}

// custom percentage procedure for *arr apps.
func percent(size, total float64) int {
	const oneHundred = 100

	return int(oneHundred - (size / total * oneHundred))
}

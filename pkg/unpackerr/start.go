package unpackerr

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/davidnewhall/unpackerr/pkg/ui"
	flag "github.com/spf13/pflag"
	"golift.io/cnfg"
	"golift.io/rotatorr"
	"golift.io/version"
	"golift.io/xtractr"
)

const (
	defaultMaxRetries  = 3
	defaultFileMode    = 0o644
	defaultDirMode     = 0o755
	defaultTimeout     = 10 * time.Second
	minimumInterval    = 15 * time.Second
	defaultInterval    = 2 * time.Minute
	cleanerInterval    = 5 * time.Second
	defaultRetryDelay  = 5 * time.Minute
	defaultStartDelay  = time.Minute
	minimumDeleteDelay = time.Second
	defaultDeleteDelay = 5 * time.Minute
	defaultHistory     = 10             // items keps in history.
	suffix             = "_unpackerred" // suffix for unpacked folders.
	mebiByte           = 1024 * 1024    // Used to turn bytes in MiB.
	updateChanBuf      = 100            // Size of xtractr callback update channels.
	defaultFolderBuf   = 20000          // Channel queue size for file system events.
	minimumFolderBuf   = 1000           // Minimum size of the folder event buffer.
	defaultLogFileMb   = 10
	defaultLogFiles    = 10
	helpLink           = "GoLift Discord: https://golift.io/discord" // prints on start and on exit.
	windows            = "windows"
)

// Unpackerr stores all the running data.
type Unpackerr struct {
	*Flags
	*Config
	*History
	*xtractr.Xtractr
	folders  *Folders
	sigChan  chan os.Signal
	updates  chan *xtractr.Response
	hookChan chan *hookQueueItem
	delChan  chan []string
	workChan chan *workThread
	*Logger
	rotatorr *rotatorr.Logger
	menu     map[string]ui.MenuItem
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
	Items    []string
	Finished uint
	Retries  uint
	Map      map[string]*Extract
}

// New returns an UnpackerPoller struct full of defaults.
// An empty struct will surely cause you pain, so use this!
func New() *Unpackerr {
	return &Unpackerr{
		Flags:    &Flags{EnvPrefix: "UN"},
		hookChan: make(chan *hookQueueItem, updateChanBuf),
		delChan:  make(chan []string, updateChanBuf),
		sigChan:  make(chan os.Signal),
		workChan: make(chan *workThread, 1),
		History:  &History{Map: make(map[string]*Extract)},
		updates:  make(chan *xtractr.Response, updateChanBuf),
		menu:     make(map[string]ui.MenuItem),
		Config: &Config{
			KeepHistory: defaultHistory,
			LogQueues:   cnfg.Duration{Duration: time.Minute},
			MaxRetries:  defaultMaxRetries,
			LogFiles:    defaultLogFiles,
			Timeout:     cnfg.Duration{Duration: defaultTimeout},
			Interval:    cnfg.Duration{Duration: defaultInterval},
			RetryDelay:  cnfg.Duration{Duration: defaultRetryDelay},
			StartDelay:  cnfg.Duration{Duration: defaultStartDelay},
			DeleteDelay: cnfg.Duration{Duration: defaultDeleteDelay},
		},
		Logger: &Logger{Logger: log.New(ioutil.Discard, "", 0)},
	}
}

// Start runs the app.
func Start() (err error) {
	log.SetFlags(log.LstdFlags) // in case we throw an error for main.go before logging is setup.

	u := New().ParseFlags() // Grab CLI args (like config file location).
	if u.Flags.verReq {
		fmt.Println(version.Print("unpackerr")) //nolint:forbidigo

		return nil // don't run anything else.
	}

	fm, dm, msg, err := u.unmarshalConfig()
	if err != nil {
		return fmt.Errorf("%s: %w", msg, err)
	}
	// Do not do any logging before this.
	// ie. No running of u.Debugf or u.Print* before running unmarshalConfig()

	// We cannot log anything until setupLogging() runs.
	// We cannot run setupLogging until we read the above config.
	u.setupLogging()
	u.Printf("Unpackerr v%s Starting! (PID: %v) %v", version.Version, os.Getpid(), version.Started)

	if err := u.validateApps(); err != nil {
		return err
	}

	u.logStartupInfo(msg)

	if u.Flags.webhook > 0 {
		return u.sampleWebhook(ExtractStatus(u.Flags.webhook))
	}

	u.Xtractr = xtractr.NewQueue(&xtractr.Config{
		Parallel: int(u.Parallel),
		Suffix:   suffix,
		Logger:   u.Logger,
		FileMode: os.FileMode(fm),
		DirMode:  os.FileMode(dm),
	})

	if len(u.Webhook) > 0 || len(u.Cmdhook) > 0 {
		go u.watchCmdAndWebhooks()
	}

	go u.watchDeleteChannel()
	go u.Run()
	u.watchWorkThread()
	u.startTray() // runs tray or waits for exit depending on hasGUI.

	return nil
}

func (u *Unpackerr) watchDeleteChannel() {
	for f := range u.delChan {
		if len(f) > 0 && f[0] != "" {
			u.DeleteFiles(f...)
		}
	}
}

func (u *Unpackerr) watchCmdAndWebhooks() {
	for qh := range u.hookChan {
		if qh.WebhookConfig.URL != "" {
			u.sendWebhookWithLog(qh.WebhookConfig, qh.WebhookPayload)
		}

		if qh.WebhookConfig.Command != "" {
			u.runCmdhookWithLog(qh.WebhookConfig, qh.WebhookPayload)
		}
	}
}

// ParseFlags turns CLI args into usable data.
func (u *Unpackerr) ParseFlags() *Unpackerr {
	flag.Usage = func() {
		fmt.Println("Usage: unpackerr [--config=filepath] [--version]") //nolint:forbidigo
		flag.PrintDefaults()
	}

	flag.StringVarP(&u.Flags.ConfigFile, "config", "c", os.Getenv("UN_CONFIG_FILE"), "Poller Config File (TOML Format)")
	flag.StringVarP(&u.Flags.EnvPrefix, "prefix", "p", "UN", "Environment Variable Prefix")
	flag.UintVarP(&u.Flags.webhook, "webhook", "w", 0, "Send test webhook. Valid values: 1,2,3,4,5,6,7,8")
	flag.BoolVarP(&u.Flags.verReq, "version", "v", false, "Print the version and exit.")
	flag.Parse()

	return u // so you can chain into ParseConfig.
}

// Run starts the loop that does the work.
func (u *Unpackerr) Run() {
	var (
		poller  = time.NewTicker(u.Config.Interval.Duration)  // poll apps at configured interval.
		cleaner = time.NewTicker(cleanerInterval)             // clean at a fast interval.
		logger  = time.NewTicker(u.Config.LogQueues.Duration) // log queue states every minute.
	)

	u.PollFolders()       // This initializes channel(s) used below.
	u.retrieveAppQueues() // Get in-app queues on startup.

	// one go routine to rule them all.
	for {
		select {
		case <-cleaner.C:
			// Check for extraction state changes and act on them.
			u.checkExtractDone()
			u.checkFolderStats()
		case <-poller.C:
			// polling interval. pull queue data from all apps.
			u.retrieveAppQueues()
			// check for state changes in the qpp queues.
			u.checkQueueChanges()
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

// custom percentage procedure for *arr apps.
func percent(size, total float64) int {
	const oneHundred = 100

	return int(oneHundred - (size / total * oneHundred))
}

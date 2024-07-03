package unpackerr

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/ui"
	"github.com/hako/durafmt"
	flag "github.com/spf13/pflag"
	"golift.io/cnfg"
	"golift.io/cnfgfile"
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
	defaultHistory     = 10             // items kept in history.
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

//nolint:gochecknoglobals
var durafmtUnits, _ = durafmt.DefaultUnitsCoder.Decode("year,week,day,hour,min,sec,ms:ms,µs:µs")

// Unpackerr stores all the running data.
type Unpackerr struct {
	*Flags
	*Config
	*History
	*xtractr.Xtractr
	metrics  *metrics
	folders  *Folders
	sigChan  chan os.Signal
	updates  chan *xtractr.Response
	hookChan chan *hookQueueItem
	delChan  chan *fileDeleteReq
	workChan chan *workThread
	*Logger
	rotatorr *rotatorr.Logger
	menu     map[string]ui.MenuItem
}

type fileDeleteReq struct {
	Paths            []string
	PurgeEmptyParent bool
}

// Logger provides a struct we can pass into other packages.
type Logger struct {
	HTTP  *log.Logger
	Info  *log.Logger
	Error *log.Logger
	Debug *log.Logger
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
		delChan:  make(chan *fileDeleteReq, updateChanBuf),
		sigChan:  make(chan os.Signal),
		workChan: make(chan *workThread, 1),
		History:  &History{Map: make(map[string]*Extract)},
		updates:  make(chan *xtractr.Response, updateChanBuf),
		menu:     make(map[string]ui.MenuItem),
		Config: &Config{
			KeepHistory: defaultHistory,
			LogQueues:   cnfg.Duration{Duration: time.Minute + time.Second},
			MaxRetries:  defaultMaxRetries,
			LogFiles:    defaultLogFiles,
			Timeout:     cnfg.Duration{Duration: defaultTimeout},
			Interval:    cnfg.Duration{Duration: defaultInterval},
			RetryDelay:  cnfg.Duration{Duration: defaultRetryDelay},
			StartDelay:  cnfg.Duration{Duration: defaultStartDelay},
			DeleteDelay: cnfg.Duration{Duration: defaultDeleteDelay},
			Webserver: &WebServer{
				Metrics:    false,
				LogFiles:   defaultLogFiles,
				LogFileMb:  defaultLogFileMb,
				ListenAddr: "0.0.0.0:5656",
				URLBase:    "/",
			},
		},
		Logger: &Logger{
			HTTP:  log.New(io.Discard, "", 0),
			Info:  log.New(io.Discard, "[INFO] ", log.LstdFlags),
			Error: log.New(io.Discard, "[ERROR] ", log.LstdFlags),
			Debug: log.New(io.Discard, "[DEBUG] ", log.Lshortfile|log.Lmicroseconds|log.Ldate),
		},
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
	u.Printf("Unpackerr v%s-%s Starting! PID: %v, UID: %d, GID: %d, Now: %v",
		version.Version, version.Revision, os.Getpid(),
		os.Getuid(), os.Getgid(), version.Started.Round(time.Second))
	u.Debugf(strings.Join(strings.Fields(strings.ReplaceAll(version.Print("unpackerr"), "\n", ", ")), " "))

	output, err := cnfgfile.Parse(u.Config, &cnfgfile.Opts{Name: "Unpackerr"})
	if err != nil {
		return fmt.Errorf("using filepath: %w", err)
	}

	if err := u.validateApps(); err != nil {
		return err
	}

	u.logStartupInfo(msg, output)

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
	u.startWebServer()
	u.watchWorkThread()
	u.startTray() // runs tray or waits for exit depending on hasGUI.

	return nil
}

func (u *Unpackerr) watchDeleteChannel() {
	for f := range u.delChan {
		if len(f.Paths) > 0 && f.Paths[0] != "" {
			u.DeleteFiles(f.Paths...)

			if !f.PurgeEmptyParent {
				continue
			}

			for _, path := range f.Paths {
				if p := filepath.Dir(path); dirIsEmpty(p) {
					u.Printf("Purging empty folder: %s", p)
					u.DeleteFiles(p)
				}
			}
		}
	}
}

func dirIsEmpty(name string) bool {
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()

	_, err = f.Readdirnames(1)

	return err == io.EOF //nolint:errorlint // this is still correct.
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
		xtractr = time.NewTicker(u.Config.StartDelay.Duration)
	)

	u.PollFolders()       // This initializes channel(s) used below.
	u.retrieveAppQueues() // Get in-app queues on startup.

	// one go routine to rule them all.
	for {
		select {
		case <-xtractr.C:
			// Check if any completed items have elapsed their start delay.
			u.extractCompletedDownloads()
		case <-poller.C:
			// polling interval. pull queue data from all apps.
			u.retrieveAppQueues()
			// check for state changes in the qpp queues.
			u.checkQueueChanges()
		case <-cleaner.C:
			// Check for extraction state changes and act on them.
			u.checkExtractDone()
			u.checkFolderStats()
		case resp := <-u.updates:
			// xtractr callback for arr app download extraction.
			u.handleXtractrCallback(resp)
		case resp := <-u.folders.Updates:
			// xtractr callback for a watched folder extraction.
			u.folderXtractrCallback(resp)
		case event := <-u.folders.Events:
			// file system event for watched folder.
			u.processEvent(event)
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

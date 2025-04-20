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
	defaultLogFileMode = 0o600
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
	bits8              = 8
	base32             = 32
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
	progress chan *Progress
	hookChan chan *hookQueueItem
	delChan  chan *fileDeleteReq
	workChan chan []func()
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

// New returns an UnpackerPoller struct full of defaults.
// An empty struct will surely cause you pain, so use this!
func New() *Unpackerr {
	return &Unpackerr{
		Flags:    &Flags{EnvPrefix: "UN"},
		hookChan: make(chan *hookQueueItem, updateChanBuf),
		delChan:  make(chan *fileDeleteReq, updateChanBuf),
		sigChan:  make(chan os.Signal),
		workChan: make(chan []func(), 1),
		History:  &History{Map: make(map[string]*Extract)},
		updates:  make(chan *xtractr.Response, updateChanBuf),
		progress: make(chan *Progress),
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
//
//nolint:gosec // not too concerned with possible integer overflows reading user-provided config files.
func Start() error {
	log.SetFlags(log.LstdFlags) // in case we throw an error for main.go before logging is setup.

	unpackerr := New().ParseFlags() // Grab CLI args (like config file location).
	if unpackerr.Flags.verReq {
		fmt.Println(version.Print("unpackerr")) //nolint:forbidigo
		return nil                              // don't run anything else.
	}

	fileMode, dirMode, msg, err := unpackerr.unmarshalConfig()
	if err != nil {
		return fmt.Errorf("%s: %w", msg, err)
	}
	// We cannot log anything until setupLogging() runs.
	// We cannot run setupLogging until we unmarshal the above config.
	unpackerr.setupLogging()
	// Do not do any logging before this.
	// ie. No running of u.Debugf or u.Print* before running setupLogging()
	unpackerr.Printf("Unpackerr v%s-%s Starting! PID: %v, UID: %d, GID: %d, Umask: %d, Now: %v",
		version.Version, version.Revision, os.Getpid(),
		os.Getuid(), os.Getgid(), getUmask(), version.Started.Round(time.Second))
	unpackerr.Debugf("%s", strings.Join(strings.Fields(strings.ReplaceAll(version.Print("unpackerr"), "\n", ", ")), " "))
	// Parse filepath: strings from the config and read in extra config files.
	output, err := cnfgfile.Parse(unpackerr.Config, &cnfgfile.Opts{
		Name:          "Unpackerr",
		TransformPath: expandHomedir,
		Prefix:        "filepath:",
	})
	if err != nil {
		return fmt.Errorf("parsing filepaths: %w", err)
	}

	if err := unpackerr.validateApps(); err != nil {
		return err
	}

	unpackerr.logStartupInfo(msg, output)

	if unpackerr.Flags.webhook > 0 {
		return unpackerr.sampleWebhook(ExtractStatus(unpackerr.Flags.webhook))
	}

	unpackerr.Xtractr = xtractr.NewQueue(&xtractr.Config{
		Parallel: int(unpackerr.Parallel),
		Suffix:   suffix,
		Logger:   unpackerr.Logger,
		FileMode: os.FileMode(fileMode),
		DirMode:  os.FileMode(dirMode),
	})

	if len(unpackerr.Webhook) > 0 || len(unpackerr.Cmdhook) > 0 {
		go unpackerr.watchCmdAndWebhooks()
	}

	go unpackerr.watchDeleteChannel()
	unpackerr.startWebServer()
	unpackerr.watchWorkThread()
	unpackerr.startTray() // runs tray or waits for exit depending on hasGUI.

	return nil
}

func fileList(paths ...string) []string {
	files := []string{}

	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			names, _ := file.Readdirnames(0)
			files = append(files, names...)
		}
	}

	return files
}

func (u *Unpackerr) watchDeleteChannel() {
	for input := range u.delChan {
		u.Debugf("Deleting files: %s", strings.Join(fileList(input.Paths...), ", "))

		if len(input.Paths) > 0 && input.Paths[0] != "" {
			u.DeleteFiles(input.Paths...)

			if !input.PurgeEmptyParent {
				continue
			}

			for _, path := range input.Paths {
				if dir := filepath.Dir(path); dirIsEmpty(dir) {
					u.Printf("Purging empty folder: %s", dir)
					u.DeleteFiles(dir)
				}
			}
		}
	}
}

func dirIsEmpty(path string) bool {
	dir, err := os.Open(path)
	if err != nil {
		return false
	}
	defer dir.Close()

	_, err = dir.Readdirnames(1)

	return err == io.EOF //nolint:errorlint // this is still correct.
}

func (u *Unpackerr) watchCmdAndWebhooks() {
	for hook := range u.hookChan {
		if hook.WebhookConfig.URL != "" {
			u.sendWebhookWithLog(hook.WebhookConfig, hook.WebhookPayload)
		}

		if hook.WebhookConfig.Command != "" {
			u.runCmdhookWithLog(hook.WebhookConfig, hook.WebhookPayload)
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
		poller  = time.NewTicker(u.Config.Interval.Duration)   // poll apps at configured interval.
		cleaner = time.NewTicker(cleanerInterval)              // clean at a fast interval.
		logger  = time.NewTicker(u.Config.LogQueues.Duration)  // log queue states every minute.
		xtractr = time.NewTicker(u.Config.StartDelay.Duration) // Check if an extract needs to start.
		now     = version.Started                              // Used for file system event time stamps.
	)

	u.PollFolders()          // This initializes channel(s) used below.
	u.retrieveAppQueues(now) // Get in-app queues on startup.

	// This is the "main go routine" in start.go.
	for {
		select {
		case now = <-poller.C:
			// polling interval. pull queue data from all apps.
			u.retrieveAppQueues(now)
			// check for state changes in the qpp queues.
			u.checkQueueChanges(now)
		case now = <-xtractr.C:
			// Check if any completed items have elapsed their start delay.
			u.extractCompletedDownloads(now)
		case now = <-cleaner.C:
			// Check for extraction state changes and act on them.
			u.checkExtractDone(now)
			u.checkFolderStats(now)
		case resp := <-u.updates:
			// xtractr callback for starr download extraction.
			u.handleXtractrCallback(resp)
		case resp := <-u.folders.Updates:
			// xtractr callback for a watched folder extraction.
			u.folderXtractrCallback(resp)
		case event := <-u.folders.Events:
			// file system event for watched folder.
			u.processEvent(event, now)
		case now := <-logger.C:
			// Log/print current queue counts once in a while.
			u.logCurrentQueue(now)
		case prog := <-u.progress:
			// Progress update for starr app extraction.
			u.handleProgress(prog)
		}
	}
}

// Custom percentage procedure for starr apps.
// Returns an unsigned integer 0-100.
func percent(remaining, total float64) uint {
	const oneHundred = 100.0

	if remaining == 0 {
		return oneHundred
	}

	return uint(oneHundred - (remaining / total * oneHundred))
}

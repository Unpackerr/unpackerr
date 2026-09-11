package unpackerr

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"github.com/Unpackerr/unpackerr/pkg/hooks"
	"github.com/Unpackerr/unpackerr/pkg/ui"
	flag "github.com/spf13/pflag"
	"golift.io/cnfg"
	"golift.io/cnfgfile"
	"golift.io/rotatorr"
	"golift.io/version"
	"golift.io/xtractr"
)

const (
	defaultMaxRetries       = 2    // two retries after the first try (3 attempts).
	defaultMaxFiles         = 1000 // Starr cap. Folders default to 0 (unlimited).
	defaultMaxRatio         = 5.0  // Starr cap. Folders default to 0 (unlimited).
	defaultSonarrMaxBytes   = "20GB"
	defaultRadarrMaxBytes   = "75GB"
	defaultLidarrMaxBytes   = "4GB"
	defaultReadarrMaxBytes  = "1GB"
	defaultWhisparrMaxBytes = "20GB"
	defaultMaxNested        = 8 // Starr extras cap. Folders default to 0 (unlimited).
	defaultExtrasMaxDepth   = 3 // Starr extras walk. Folders default to 0 (unlimited).
	defaultFileMode         = 0o644
	defaultLogFileMode      = 0o600
	defaultDirMode          = 0o755
	defaultTimeout          = 10 * time.Second
	minimumInterval         = 15 * time.Second
	defaultInterval         = 2 * time.Minute
	cleanerInterval         = 5 * time.Second
	defaultRetryDelay       = 5 * time.Minute
	defaultStartDelay       = time.Minute
	minimumDeleteDelay      = time.Second
	defaultDeleteDelay      = 5 * time.Minute
	staleItemTimeout        = 24 * time.Hour // Safety net: items stuck at intermediate states are cleaned up.
	defaultHistory          = 200            // JSONL cap; tray still shows trayHistory names.
	trayHistory             = 10             // items kept in the GUI history menu.
	suffix                  = "_unpackerred" // suffix for unpacked folders.
	updateChanBuf           = 100            // Size of xtractr callback update channels.
	signalBuf               = 4              // Hold HUP/TERM until waitForExit starts.
	defaultFolderBuf        = 20000          // Channel queue size for file system events.
	minimumFolderBuf        = 1000           // Minimum size of the folder event buffer.
	defaultLogFileMb        = 10
	defaultLogFiles         = 10
	helpLink                = "GoLift Discord: https://golift.io/discord" // prints on start and on exit.
	windows                 = "windows"
	bits8                   = 8
	base32                  = 32
)

// Unpackerr stores all the running data.
type Unpackerr struct {
	*Flags
	*Config
	*History
	*xtractr.Xtractr
	metrics    *metrics
	folders    *Folders
	sigChan    chan os.Signal
	updates    chan *xtractr.Response
	progChan   chan *ExtractProgress
	hookWorker *hooks.Worker
	delChan    chan *fileDeleteReq
	taskChan   chan *mainTask // HTTP hands config applies and queue actions to Run().
	workChan   chan []func()
	*Logger
	rotatorr *rotatorr.Logger
	httpLog  *rotatorr.Logger
	menu     map[string]ui.MenuItem
	// Live Config is owned by the main goroutine in Run(). fileConfig is the
	// on-disk shape (filepath: values kept) and is also written by the tray,
	// so it and the hook slices that /api/stats counts sit under configMu.
	fileConfig       *Config
	envUsed          map[string]string // UN_* suffixes that ParseENV wrote; immutable after startup
	livePasswords    StringSlice       // post-env, pre-expansion; GET /live uses this
	configMu         sync.RWMutex
	tickers          *loopTickers
	pendingRestart   bool
	inFlight         atomic.Int64 // queued-or-running delete and hook work.
	workThreads      int
	hookOnce         sync.Once
	uiPassMu         sync.RWMutex // live webserver auth: UIPassword, APIKeys, Roles, keyPerms, Upstreams, allow
	uiPasswordNotice string
	uiPasswordGenErr error
	configWriteErr   error
	adminKeyNotice   string
	adminKeyErr      error
	histPath         string
	histMu           sync.Mutex // records and the JSONL file; HTTP reads, main loop appends.
	histLines        int        // lines in the file since the last compaction.
	records          []HistoryRecord
}

type fileDeleteReq struct {
	Paths            []string
	PurgeEmptyParent bool
	// PurgeEmptyRoot, when set with PurgeEmptyParent, allows purging empty parent dirs
	// up to and including this path (e.g. the Starr app download folder). Stops above this root.
	PurgeEmptyRoot string
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
	reset      bool
}

// New returns an UnpackerPoller struct full of defaults.
// An empty struct will surely cause you pain, so use this!
func New() *Unpackerr {
	return &Unpackerr{
		Flags:      &Flags{EnvPrefix: "UN"},
		hookWorker: hooks.NewWorker(updateChanBuf),
		delChan:    make(chan *fileDeleteReq, updateChanBuf),
		taskChan:   make(chan *mainTask, updateChanBuf),
		sigChan:    make(chan os.Signal, signalBuf),
		workChan:   make(chan []func(), 1),
		History:    &History{Map: make(map[string]*Extract), forgotten: make(map[string]struct{})},
		folders:    &Folders{Folders: make(map[string]*Folder)}, // replaced by PollFolders when folders are configured.
		updates:    make(chan *xtractr.Response, updateChanBuf),
		progChan:   make(chan *ExtractProgress),
		menu:       make(map[string]ui.MenuItem),
		Config: &Config{
			KeepHistory:   defaultHistory,
			LogQueues:     cnfg.Duration{Duration: time.Minute + time.Second},
			MaxRetries:    defaultMaxRetries,
			RemnantAction: remnantAction(""),
			LogFiles:      defaultLogFiles,
			Timeout:       cnfg.Duration{Duration: defaultTimeout},
			Interval:      cnfg.Duration{Duration: defaultInterval},
			RetryDelay:    cnfg.Duration{Duration: defaultRetryDelay},
			StartDelay:    cnfg.Duration{Duration: defaultStartDelay},
			DeleteDelay:   cnfg.Duration{Duration: defaultDeleteDelay},
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
//nolint:gosec,funlen // not too concerned with possible integer overflows reading user-provided config files.
func Start() error {
	log.SetFlags(log.LstdFlags) // in case we throw an error for main.go before logging is setup.

	unpackerr := New()
	notifySignals(unpackerr.sigChan)
	unpackerr.ParseFlags() // Grab CLI args (like config file location).

	if unpackerr.verReq {
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

	if err := unpackerr.handleStartupPassword(); err != nil {
		return err
	}

	if unpackerr.reset {
		return nil
	}

	unpackerr.loadHistory()
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

	if unpackerr.webhook > 0 {
		return unpackerr.sampleWebhook(ExtractStatus(unpackerr.webhook))
	}

	unpackerr.Xtractr = xtractr.NewQueue(&xtractr.Config{
		Parallel: int(unpackerr.Parallel),
		Suffix:   suffix,
		Logger:   unpackerr.Logger,
		FileMode: os.FileMode(fileMode),
		DirMode:  os.FileMode(dirMode),
	})

	unpackerr.ensureHookWorker()

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
			_ = file.Close()

			files = append(files, names...)
		}
	}

	return files
}

// queueDelete publishes a delete request and counts it in flight. Counting at
// the send keeps it atomic with respect to idle(): both run on the main loop,
// so a restart can never observe the gap between a send and its receive.
func (u *Unpackerr) queueDelete(req *fileDeleteReq) {
	u.inFlight.Add(1)

	u.delChan <- req
}

func (u *Unpackerr) watchDeleteChannel() {
	for input := range u.delChan {
		u.deleteRequest(input)
	}
}

func (u *Unpackerr) deleteRequest(input *fileDeleteReq) {
	defer u.inFlight.Add(-1) // paired with queueDelete.

	if len(input.Paths) == 0 {
		return
	}

	u.Debugf("Deleting files: %s", strings.Join(fileList(input.Paths...), ", "))
	u.DeleteFiles(input.Paths...)

	if !input.PurgeEmptyParent {
		return
	}

	root := input.PurgeEmptyRoot
	if root != "" {
		root = filepath.Clean(root)
	}

	if purged := u.purgeEmptyFolders(input.Paths, root); purged > 0 {
		if root != "" {
			u.Printf("Purged %d empty folder(s) up to %s", purged, root)
		} else {
			u.Printf("Purged %d empty folder(s)", purged)
		}
	}
}

// purgeEmptyFolders removes empty ancestor dirs of the given paths, up to root (if set).
// Each dir is considered at most once; dirs are purged deepest-first. Returns the number removed.
func (u *Unpackerr) purgeEmptyFolders(paths []string, root string) int {
	// Collect unique candidate dirs (all ancestors of all paths, up to root).
	candidates := make(map[string]struct{})

	for _, path := range paths {
		if path == "" {
			continue
		}

		for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
			dir = filepath.Clean(dir)
			if root != "" {
				rel, err := filepath.Rel(root, dir)
				if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					break
				}
			}

			candidates[dir] = struct{}{}

			if root != "" && dir == root {
				break
			}

			if parent := filepath.Dir(dir); parent == dir {
				break
			}
		}
	}

	if len(candidates) == 0 {
		return 0
	}

	// Sort deepest first so we remove children before parents.
	dirs := make([]string, 0, len(candidates))
	for d := range candidates {
		dirs = append(dirs, d)
	}

	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})

	var purged int

	for _, dir := range dirs {
		if dirIsEmpty(dir) {
			u.DeleteFiles(dir)

			purged++
		}
	}

	return purged
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

func (u *Unpackerr) ensureHookWorker() {
	u.hookOnce.Do(func() {
		go u.hookWorker.Run(u.Logger, func() { u.inFlight.Add(-1) })
	})
}

// queueHook publishes a hook and counts it in flight. See queueDelete.
func (u *Unpackerr) queueHook(item *hooks.Item) {
	u.inFlight.Add(1)

	u.hookWorker.Enqueue(item)
}

// ParseFlags turns CLI args into usable data.
func (u *Unpackerr) ParseFlags() *Unpackerr {
	flag.Usage = func() {
		fmt.Println("Usage: unpackerr [--config=filepath] [--version] [--reset]") //nolint:forbidigo
		flag.PrintDefaults()
	}

	flag.StringVarP(&u.ConfigFile, "config", "c", os.Getenv("UN_CONFIG_FILE"), "Poller Config File (TOML Format)")
	flag.StringVarP(&u.EnvPrefix, "prefix", "p", "UN", "Environment Variable Prefix")
	flag.UintVarP(&u.webhook, "webhook", "w", 0, "Send test webhook. Valid values: 1,2,3,4,5,6,7,8")
	flag.BoolVarP(&u.verReq, "version", "v", false, "Print the version and exit.")
	flag.BoolVar(&u.reset, "reset", false, "Reset the web UI password, write it to the config file, and exit")
	flag.Parse()

	return u // so you can chain into ParseConfig.
}

// Run starts the loop that does the work.
// loopTickers are owned by Run(). A general config PUT resets them in place.
type loopTickers struct {
	poller   *time.Ticker // poll apps at configured interval.
	xtractr  *time.Ticker // check if an extract needs to start.
	progress *time.Ticker // progress update for extractions.
	logger   *time.Ticker // log/print current queue counts.
}

func (u *Unpackerr) resetTickers() {
	if u.tickers == nil {
		return // PUT before Run(); tests do this.
	}

	u.tickers.poller.Reset(u.Interval.Duration)
	u.tickers.xtractr.Reset(u.StartDelay.Duration)
	u.tickers.progress.Reset(u.Progress.Duration)
	u.tickers.logger.Reset(u.LogQueues.Duration)
}

func (u *Unpackerr) Run() {
	u.tickers = &loopTickers{
		poller:   time.NewTicker(u.Interval.Duration),
		xtractr:  time.NewTicker(u.StartDelay.Duration),
		progress: time.NewTicker(u.Progress.Duration),
		logger:   time.NewTicker(u.LogQueues.Duration),
	}

	cleaner := time.NewTicker(cleanerInterval) // clean at a fast interval.
	now := version.Started                     // Used for file system event time stamps.

	if u.starrAppCount()+len(u.Folders) == 0 {
		u.Printf("No Starr apps or folders configured. Shut down and add some apps or folders to your config file.")
	}

	u.PollFolders()          // This initializes channel(s) used below.
	u.retrieveAppQueues(now) // Get in-app queues on startup.

	// This is the "main go routine" in start.go.
	for {
		select {
		case now = <-u.tickers.poller.C:
			// polling interval. pull queue data from all apps.
			u.retrieveAppQueues(now)
			// check for state changes in the qpp queues.
			u.checkQueueChanges(now)
		case now = <-u.tickers.xtractr.C:
			// Check if any completed items have elapsed their start delay.
			u.extractCompletedDownloads(now)
		case now = <-cleaner.C:
			// Check for extraction state changes and act on them.
			u.checkExtractDone(now)
			u.checkFolderStats(now)
			u.maybeRestart()
		case resp := <-u.updates:
			// xtractr callback for starr download extraction.
			u.handleXtractrCallback(resp)
		case resp := <-u.folders.Updates:
			// xtractr callback for a watched folder extraction.
			u.folderXtractrCallback(resp)
		case event := <-u.folders.Events:
			// file system event for watched folder.
			u.processEvent(event, now)
		case task := <-u.taskChan:
			// HTTP config PUT and queue retry/forget mutate live state on this goroutine.
			task.result <- task.fn()
		case now := <-u.tickers.logger.C:
			// Log/print current queue counts once in a while, when something is configured.
			if u.starrAppCount()+len(u.Folders) > 0 {
				u.logCurrentQueue(now)
			}
		case prog := <-u.progChan:
			// Update progress for in-process extractions.
			u.handleProgress(prog)
		case now = <-u.tickers.progress.C:
			// Print the collected progress info.
			u.printProgress(now)
		}
	}
}

func (u *Unpackerr) maxRetries() uint {
	if u.MaxRetries == 0 {
		return defaultMaxRetries
	}

	return u.MaxRetries
}

var errInvalidMaxBytes = errors.New("invalid max_bytes")

func parseOptionalMaxBytes(size string) (uint64, bool, error) {
	if strings.TrimSpace(size) == "" {
		return 0, false, nil
	}

	n, err := parseExtractMaxBytes(size)
	if err != nil {
		return 0, false, err
	}

	return n, true, nil
}

func parseExtractMaxBytes(size string) (uint64, error) {
	size = strings.TrimSpace(size)
	if size == "" {
		return 0, fmt.Errorf("%w: empty", errInvalidMaxBytes)
	}

	if size == "0" || strings.EqualFold(size, "0B") {
		return 0, nil
	}

	n, err := bytefmt.ToBytes(size)
	if err != nil {
		return 0, fmt.Errorf("%w: %q: %w", errInvalidMaxBytes, size, err)
	}

	return n, nil
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

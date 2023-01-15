//go:build windows || darwin

package unpackerr

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/bindata"
	"github.com/Unpackerr/unpackerr/pkg/ui"
	"github.com/Unpackerr/unpackerr/pkg/update"
	"github.com/getlantern/systray"
	"github.com/hako/durafmt"
	"golift.io/version"
)

// startTray Run()s readyTray to bring up the web server and the GUI app.
func (u *Unpackerr) startTray() {
	if !ui.HasGUI() {
		go u.Run()

		signal.Notify(u.sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
		u.Printf("[unpackerr] Need help? %s\n=====> Exiting! Caught Signal: %v", helpLink, <-u.sigChan)

		return
	}

	systray.Run(u.readyTray, u.exitTray)
}

func (u *Unpackerr) exitTray() {
	u.Xtractr.Stop() // stop and wait for extractions.
	// because systray wants to control the exit code? no..
	os.Exit(0)
}

// readyTray creates the system tray/menu bar app items, and starts the web server.
func (u *Unpackerr) readyTray() {
	b, err := bindata.Asset(ui.SystrayIcon)
	if err == nil {
		systray.SetTemplateIcon(b, b)
	} else {
		u.Printf("[ERROR] Reading Icon: %v", err)
		systray.SetTitle("DNC")
	}

	systray.SetTooltip("Unpackerr" + " v" + version.Version)
	u.makeChannels()

	u.menu["info"].Disable()

	go u.watchKillerChannels()
	go u.watchDebugChannels()
	go u.Run()

	u.watchGuiChannels()
}

func (u *Unpackerr) makeChannels() {
	conf := systray.AddMenuItem("Config", "show configuration")
	u.menu["conf"] = ui.WrapMenu(conf)
	u.menu["edit"] = ui.WrapMenu(conf.AddSubMenuItem("Edit", "open configuration file"))

	link := systray.AddMenuItem("Links", "external resources")
	u.menu["link"] = ui.WrapMenu(link)
	u.menu["info"] = ui.WrapMenu(link.AddSubMenuItem("Unpackerr", version.Print("Unpackerr")))
	u.menu["disc"] = ui.WrapMenu(link.AddSubMenuItem("Go Lift Discord", "open Go Lift discord server"))
	u.menu["gh"] = ui.WrapMenu(link.AddSubMenuItem("GitHub Project", "Unpackerr on GitHub"))

	logs := systray.AddMenuItem("Logs", "log file info")
	u.menu["logs"] = ui.WrapMenu(logs)
	u.menu["logs_view"] = ui.WrapMenu(logs.AddSubMenuItem("View", "view the application log"))
	u.menu["logs_rotate"] = ui.WrapMenu(logs.AddSubMenuItem("Rotate", "rotate log file"))

	if u.Config.Debug {
		debug := systray.AddMenuItem("Debug", "Debug Menu")
		u.menu["debug"] = ui.WrapMenu(debug)
		u.menu["debug_panic"] = ui.WrapMenu(debug.AddSubMenuItem("Panic", "cause an application panic"))
	}

	// top level
	u.makeStatsChannels()
	u.makeHistoryChannels()
	u.menu["update"] = ui.WrapMenu(systray.AddMenuItem("Update", "Check GitHub for Update"))
	u.menu["exit"] = ui.WrapMenu(systray.AddMenuItem("Quit", "Exit Unpackerr"))
}

func (u *Unpackerr) watchDebugChannels() {
	if !u.Config.Debug {
		return
	}

	for {
		select {
		case <-u.menu["debug"].Clicked():
			// turn on and off debug?
			// u.menu["debug"].Check()
		case <-u.menu["debug_panic"].Clicked():
			u.Printf("User Requested Application Panic, good bye.")
			panic("user requested panic")
		}
	}
}

func (u *Unpackerr) watchGuiChannels() {
	for {
		//nolint:errcheck
		select {
		case <-u.menu["conf"].Clicked():
			// does nothing on purpose
		case <-u.menu["edit"].Clicked():
			u.Printf("User Editing Config File: %s", u.Flags.ConfigFile)
			ui.OpenFile(u.Flags.ConfigFile)
		case <-u.menu["link"].Clicked():
			// does nothing on purpose
		case <-u.menu["info"].Clicked():
			// does nothing on purpose
		case <-u.menu["disc"].Clicked():
			ui.OpenURL("https://golift.io/discord")
		case <-u.menu["gh"].Clicked():
			ui.OpenURL("https://github.com/Unpackerr/unpackerr/")
		case <-u.menu["logs"].Clicked():
			// does nothing on purpose
		case <-u.menu["logs_view"].Clicked():
			u.Printf("User Viewing Log File: %s", u.Config.LogFile)
			ui.OpenLog(u.Config.LogFile)
		case <-u.menu["logs_rotate"].Clicked():
			u.rotateLogs()
		case <-u.menu["update"].Clicked():
			u.checkForUpdate()
		}
	}
}

func (u *Unpackerr) makeHistoryChannels() {
	history := systray.AddMenuItem("History", fmt.Sprintf("display last %d items queued", u.KeepHistory))
	u.menu["history"] = ui.WrapMenu(history)
	u.menu["hist_none"] = ui.WrapMenu(history.AddSubMenuItem("-- there is no history --", "nothing has been queued yet"))
	u.menu["hist_none"].Disable()

	if u.KeepHistory == 0 {
		u.menu["hist_none"].SetTitle("-- history disabled --")
		u.menu["hist_none"].SetTooltip("history is disabled in the config")
	}

	for i := 0; i < int(u.KeepHistory); i++ {
		u.menu["hist_"+strconv.Itoa(i)] = ui.WrapMenu(history.AddSubMenuItem("", ""))
		u.menu["hist_"+strconv.Itoa(i)].Disable()
		u.menu["hist_"+strconv.Itoa(i)].Hide()
	}
}

func (u *Unpackerr) makeStatsChannels() {
	stats := systray.AddMenuItem("Stats", "")
	u.menu["stats"] = ui.WrapMenu(stats)
	ui.WrapMenu(stats.AddSubMenuItem("-- counters --", "these counters reset as data is processed")).Disable()
	u.menu["stats_stacks"] = ui.WrapMenu(stats.AddSubMenuItem("Stacks: 0", "internal loop stack depth"))
	u.menu["stats_waiting"] = ui.WrapMenu(stats.AddSubMenuItem("Waiting: 0", "unprocessed items in starr apps"))
	u.menu["stats_queued"] = ui.WrapMenu(stats.AddSubMenuItem("Queued: 0", "items queued for extraction"))
	u.menu["stats_extracting"] = ui.WrapMenu(stats.AddSubMenuItem("Extracting: 0 ", "items currently extracting"))
	u.menu["stats_failed"] = ui.WrapMenu(stats.AddSubMenuItem("Failed: 0", "failed extractions"))
	u.menu["stats_extracted"] = ui.WrapMenu(stats.AddSubMenuItem("Extracted: 0", "items extracted, not imported"))
	u.menu["stats_imported"] = ui.WrapMenu(stats.AddSubMenuItem("Imported: 0", "items extracted AND imported"))
	u.menu["stats_deleted"] = ui.WrapMenu(stats.AddSubMenuItem("Deleted: 0", "items imported and deleted"))
	ui.WrapMenu(stats.AddSubMenuItem("-- totals --", "these increment until the app is stopped")).Disable()
	u.menu["stats_finished"] = ui.WrapMenu(stats.AddSubMenuItem("Finished: 0", "total items processed and completed"))
	u.menu["stats_retries"] = ui.WrapMenu(stats.AddSubMenuItem("Retries: 0", "total times an item was restarted"))
	u.menu["stats_hookOK"] = ui.WrapMenu(stats.AddSubMenuItem("Webhooks: 0", "webhooks sent"))
	u.menu["stats_hookFail"] = ui.WrapMenu(stats.AddSubMenuItem("Hook Errors: 0", "webhooks failed to send"))

	u.menu["stats_waiting"].Disable()
	u.menu["stats_queued"].Disable()
	u.menu["stats_extracting"].Disable()
	u.menu["stats_failed"].Disable()
	u.menu["stats_extracted"].Disable()
	u.menu["stats_imported"].Disable()
	u.menu["stats_deleted"].Disable()
	u.menu["stats_finished"].Disable()
	u.menu["stats_retries"].Disable()
	u.menu["stats_hookOK"].Disable()
	u.menu["stats_hookFail"].Disable()
	u.menu["stats_stacks"].Disable()
}

func (u *Unpackerr) updateTray(
	retries,
	finished,
	waiting,
	queued,
	extracting,
	failed,
	extracted,
	imported,
	deleted,
	hookOK,
	hookFail,
	stacks uint,
) {
	if !ui.HasGUI() {
		return
	}

	const baseTen = 10

	u.menu["stats_waiting"].SetTitle("Waiting: " + strconv.FormatUint(uint64(waiting), baseTen))
	u.menu["stats_queued"].SetTitle("Queued: " + strconv.FormatUint(uint64(queued), baseTen))
	u.menu["stats_extracting"].SetTitle("Extracting: " + strconv.FormatUint(uint64(extracting), baseTen))
	u.menu["stats_failed"].SetTitle("Failed: " + strconv.FormatUint(uint64(failed), baseTen))
	u.menu["stats_extracted"].SetTitle("Extracted: " + strconv.FormatUint(uint64(extracted), baseTen))
	u.menu["stats_imported"].SetTitle("Imported: " + strconv.FormatUint(uint64(imported), baseTen))
	u.menu["stats_deleted"].SetTitle("Deleted: " + strconv.FormatUint(uint64(deleted), baseTen))
	u.menu["stats_finished"].SetTitle("Finished: " + strconv.FormatUint(uint64(finished), baseTen))
	u.menu["stats_retries"].SetTitle("Retries: " + strconv.FormatUint(uint64(retries), baseTen))
	u.menu["stats_hookOK"].SetTitle("Webhooks: " + strconv.FormatUint(uint64(hookOK), baseTen))
	u.menu["stats_hookFail"].SetTitle("Hook Errors: " + strconv.FormatUint(uint64(hookFail), baseTen))
	u.menu["stats_stacks"].SetTitle("Loop Stacks: " + strconv.FormatUint(uint64(stacks), baseTen))
}

func (u *Unpackerr) watchKillerChannels() {
	defer systray.Quit() // this kills the app

	for {
		select {
		case sigc := <-u.sigChan:
			u.Printf("Need help? %s\n=====> Exiting! Caught Signal: %v", helpLink, sigc)
			return
		case <-u.menu["exit"].Clicked():
			u.Printf("Need help? %s\n=====> Exiting! User Requested", helpLink)
			return
		}
	}
}

func (u *Unpackerr) rotateLogs() {
	u.Printf("User Requested: Rotate Log File!")

	if _, err := u.rotatorr.Rotate(); err != nil {
		u.Printf("[ERROR] Rotating Log Files: %v", err)
	}
}

func (u *Unpackerr) checkForUpdate() {
	u.Print("User Requested: Update Check")

	switch update, err := update.Check("Unpackerr/unpackerr", version.Version); {
	case err != nil:
		u.Printf("[ERROR] Update Check: %v", err)
		_, _ = ui.Error("Unpackerr", "Failure checking version on GitHub: "+err.Error())
	case update.Outdate:
		yes, _ := ui.Question("Unpackerr", "An Update is available! Download?\n\n"+
			"Your Version: "+update.Version+"\n"+
			"New Version: "+update.Current+"\n"+
			"Date: "+update.RelDate.Format("Jan 2, 2006")+" ("+
			durafmt.Parse(time.Since(update.RelDate).Round(time.Hour)).String()+" ago)", false)
		if yes {
			_ = ui.OpenURL(update.CurrURL)
		}
	default:
		_, _ = ui.Info("Unpackerr", "You're up to date! Version: "+update.Version+"\n"+
			"Updated: "+update.RelDate.Format("Jan 2, 2006")+" ("+
			durafmt.Parse(time.Since(update.RelDate).Round(time.Hour)).String()+" ago)")
	}
}

// This is called every time an item is queued.
func (u *Unpackerr) updateHistory(item string) {
	if u.KeepHistory == 0 {
		return
	}

	if ui.HasGUI() && item != "" {
		u.menu["hist_none"].Hide()
	}

	// u.History.Items is a slice with a set (identical) length and capacity.
	for i := len(u.History.Items) - 1; i >= 0; i-- {
		if i == 0 {
			u.History.Items[0] = item
		} else {
			u.History.Items[i] = u.History.Items[i-1]
		}

		if !ui.HasGUI() {
			continue
		}

		if u.History.Items[i] != "" {
			u.menu["hist_"+strconv.Itoa(i)].SetTitle(u.History.Items[i])
			u.menu["hist_"+strconv.Itoa(i)].Show()
		} else {
			u.menu["hist_"+strconv.Itoa(i)].Hide()
		}
	}
}

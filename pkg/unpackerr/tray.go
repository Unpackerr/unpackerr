// +build windows darwin

package unpackerr

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/davidnewhall/unpackerr/pkg/bindata"
	"github.com/davidnewhall/unpackerr/pkg/ui"
	"github.com/davidnewhall/unpackerr/pkg/update"
	"github.com/getlantern/systray"
	"github.com/hako/durafmt"
	"golift.io/version"
)

// startTray Run()s readyTray to bring up the web server and the GUI app.
func (u *Unpackerr) startTray() {
	if !ui.HasGUI() {
		signal.Notify(u.sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
		u.Printf("[unpackerr] Need help? %s\n=====> Exiting! Caught Signal: %v", helpLink, <-u.sigChan)

		return
	}

	systray.Run(u.readyTray, u.exitTray)
}

func (u *Unpackerr) exitTray() {
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
	u.watchGuiChannels()
}

func (u *Unpackerr) makeChannels() {
	conf := systray.AddMenuItem("Config", "show configuration")
	u.menu["conf"] = ui.WrapMenu(conf)
	u.menu["edit"] = ui.WrapMenu(conf.AddSubMenuItem("Edit", "open configuration file"))
	// u.menu["load"] = ui.WrapMenu(conf.AddSubMenuItem("Reload", "reload configuration"))

	link := systray.AddMenuItem("Links", "external resources")
	u.menu["link"] = ui.WrapMenu(link)
	u.menu["info"] = ui.WrapMenu(link.AddSubMenuItem("Unpackerr", version.Print("Unpackerr")))
	u.menu["disc"] = ui.WrapMenu(link.AddSubMenuItem("Go Lift Discord", "open Go Lift discord server"))
	u.menu["gh"] = ui.WrapMenu(link.AddSubMenuItem("GitHub Project", "Unpackerr on GitHub"))

	logs := systray.AddMenuItem("Logs", "log file info")
	u.menu["logs"] = ui.WrapMenu(logs)
	u.menu["logs_view"] = ui.WrapMenu(logs.AddSubMenuItem("View", "view the application log"))
	u.menu["logs_rotate"] = ui.WrapMenu(logs.AddSubMenuItem("Rotate", "rotate log file"))

	// top level
	u.menu["update"] = ui.WrapMenu(systray.AddMenuItem("Update", "Check GitHub for Update"))
	u.menu["exit"] = ui.WrapMenu(systray.AddMenuItem("Quit", "Exit Unpackerr"))
}

func (u *Unpackerr) watchGuiChannels() {
	for {
		// nolint:errcheck
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
			ui.OpenURL("https://github.com/davidnewhall/unpackerr/")
		case <-u.menu["logs"].Clicked():
			// does nothing on purpose
		case <-u.menu["logs_view"].Clicked():
			u.Printf("User Viewing Log File: %s", u.Config.LogFile)
			ui.OpenLog(u.Config.LogFile)
		case <-u.menu["logs_rotate"].Clicked():
			// add rotate method, requires bubbling up logger.
			ui.Info("Unpackerr", "That button does not work yet.")
		case <-u.menu["update"].Clicked():
			u.checkForUpdate()
		}
	}
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

func (u *Unpackerr) checkForUpdate() {
	u.Print("User Requested Update Check")

	switch update, err := update.Check("davidnewhall/unpackerr", version.Version); {
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

//go:build windows || darwin

package unpackerr

import (
	"os"

	"github.com/Unpackerr/unpackerr/pkg/bindata"
	"github.com/Unpackerr/unpackerr/pkg/ui"
	"github.com/Unpackerr/unpackerr/pkg/update"
	"github.com/dromara/carbon/v2"
	"github.com/energye/systray"
	"golift.io/version"
)

// startTray Run()s readyTray to bring up the web server and the GUI app.
func (u *Unpackerr) startTray() {
	if !ui.HasGUI() {
		go u.Run()

		u.waitForExit()

		return
	}

	systray.Run(u.readyTray, u.exitTray)
}

// showTrayMenu pops the tray menu. energye/systray does not attach the menu to
// the status item the way getlantern did, so a click does nothing unless we
// call ShowMenu. Windows left-click has the same requirement.
func showTrayMenu(menu systray.IMenu) {
	if menu == nil {
		return
	}

	_ = menu.ShowMenu()
}

func (u *Unpackerr) exitTray() {
	u.Stop() // stop and wait for extractions.
	// because systray wants to control the exit code? no..
	os.Exit(0)
}

// readyTray creates the system tray/menu bar app items, and starts the web server.
func (u *Unpackerr) readyTray() {
	systray.SetTemplateIcon(bindata.SystrayIcon, bindata.SystrayIcon)
	systray.SetTooltip("Unpackerr" + " v" + version.Version)
	systray.SetOnClick(showTrayMenu)
	systray.SetOnRClick(showTrayMenu)
	u.makeChannels()

	u.menu["info"].Disable()

	go u.watchKillerChannels()
	go u.Run()

	u.showGeneratedPassword()
	u.watchGuiChannels()
}

func (u *Unpackerr) makeChannels() {
	webURL := u.Webserver.localURL()
	tip := "open the local web UI"

	if webURL != "" {
		tip = webURL
	}

	u.menu["webui"] = ui.WrapMenu(systray.AddMenuItem("WebUI", tip))
	if webURL == "" {
		u.menu["webui"].Hide()
	}

	u.menu["pass"] = ui.WrapMenu(systray.AddMenuItem("Change Password", "set the web UI password"))
	if u.Webserver == nil || !u.Webserver.Enabled() ||
		u.uiPassword().Type() != AuthPassword || u.uiPasswordEnvSet() {
		u.menu["pass"].Hide()
	}

	link := systray.AddMenuItem("Links", "external resources")
	u.menu["link"] = ui.WrapMenu(link)
	u.menu["help"] = ui.WrapMenu(link.AddSubMenuItem("Help Website", "https://unpackerr.zip"))
	u.menu["info"] = ui.WrapMenu(link.AddSubMenuItem("Unpackerr", version.Print("Unpackerr")))
	u.menu["disc"] = ui.WrapMenu(link.AddSubMenuItem("Go Lift Discord", "open Go Lift discord server"))
	u.menu["gh"] = ui.WrapMenu(link.AddSubMenuItem("GitHub Project", "Unpackerr on GitHub"))

	u.menu["logs"] = ui.WrapMenu(systray.AddMenuItem("View logs", "view the application log"))

	u.menu["update"] = ui.WrapMenu(systray.AddMenuItem("Update", "Check GitHub for Update"))
	u.menu["exit"] = ui.WrapMenu(systray.AddMenuItem("Quit", "Exit Unpackerr"))
}

func (u *Unpackerr) watchGuiChannels() {
	for {
		select {
		case <-u.menu["webui"].Clicked():
			u.openWebUI()
		case <-u.menu["pass"].Clicked():
			u.changePasswordDialog()
		case <-u.menu["link"].Clicked():
			// does nothing on purpose
		case <-u.menu["help"].Clicked():
			_ = ui.OpenURL("https://unpackerr.zip")
		case <-u.menu["info"].Clicked():
			// does nothing on purpose
		case <-u.menu["disc"].Clicked():
			_ = ui.OpenURL("https://golift.io/discord")
		case <-u.menu["gh"].Clicked():
			_ = ui.OpenURL("https://github.com/Unpackerr/unpackerr/")
		case <-u.menu["logs"].Clicked():
			u.Printf("User Viewing Log File: %s", u.LogFile)
			_ = ui.OpenLog(u.LogFile)
		case <-u.menu["update"].Clicked():
			u.checkForUpdate()
		}
	}
}

func (u *Unpackerr) openWebUI() {
	addr := u.Webserver.localURL()
	if addr == "" {
		return
	}

	u.Printf("User Opening Web UI: %s", addr)
	_ = ui.OpenURL(addr)
}

func (u *Unpackerr) watchKillerChannels() {
	defer systray.Quit() // this kills the app

	for {
		select {
		case sigc := <-u.sigChan:
			if isHangup(sigc) {
				u.reopenLogs()

				continue
			}

			u.Printf("Need help? %s\n=====> Exiting! Caught Signal: %v", helpLink, sigc)

			return
		case <-u.menu["exit"].Clicked():
			u.Printf("Need help? %s\n=====> Exiting! User Requested", helpLink)
			return
		}
	}
}

func (u *Unpackerr) checkForUpdate() {
	u.Printf("User Requested: Update Check")

	update, err := update.Check("Unpackerr/unpackerr", version.Version)
	if err != nil {
		u.Errorf("Update Check: %v", err)
		_, _ = ui.Error("Unpackerr", "Failure checking version on GitHub: %v", err)

		return
	}

	ago := carbon.CreateFromStdTime(update.RelDate).DiffAbsInString()

	if !update.Outdate {
		_, _ = ui.Info("Unpackerr", "You're up to date! Version: %s\nUpdated: %s (%s ago)",
			update.Version, update.RelDate.Format("Jan 2, 2006"), ago)

		return
	}

	yes, _ := ui.Question("Unpackerr", false,
		"An Update is available! Download?\n\nYour Version: %s\nNew Version: %s\nUpdated: %s (%s ago)",
		update.Version, update.Current, update.RelDate.Format("Jan 2, 2006"), ago)
	if yes {
		_ = ui.OpenURL(update.CurrURL)
	}
}

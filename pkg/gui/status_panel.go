package gui

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"
	"github.com/jesseduffield/lazydocker/pkg/commands"
	"github.com/jesseduffield/lazydocker/pkg/utils"
	"github.com/jesseduffield/yaml"
)

func (gui *Gui) getStatusContexts() []string {
	if gui.DockerCommand.InDockerComposeProject {
		return []string{"logs", "credits", "config"}
	}
	return []string{"credits"}
}

func (gui *Gui) getStatusContextTitles() []string {
	if gui.DockerCommand.InDockerComposeProject {
		return []string{gui.Tr.LogsTitle, gui.Tr.CreditsTitle, gui.Tr.DockerComposeConfigTitle}
	}
	return []string{gui.Tr.CreditsTitle}
}

func (gui *Gui) refreshStatus() error {
	v := gui.getStatusView()

	gui.g.Update(func(*gocui.Gui) error {
		v.Clear()
		fmt.Fprint(v, "lazydocker")
		return nil
	})

	return nil
}

func (gui *Gui) handleStatusClick(g *gocui.Gui, v *gocui.View) error {
	if gui.popupPanelFocused() {
		return nil
	}

	if _, err := gui.g.SetCurrentView(v.Name()); err != nil {
		return err
	}

	return gui.handleStatusSelect(g, v)
}

func (gui *Gui) handleStatusSelect(g *gocui.Gui, v *gocui.View) error {
	if gui.popupPanelFocused() {
		return nil
	}

	key := gui.getStatusContexts()[gui.State.Panels.Status.ContextIndex]
	if !gui.shouldRefresh(key) {
		return nil
	}

	gui.clearMainView()

	mainView := gui.getMainView()
	mainView.Tabs = gui.getStatusContextTitles()
	mainView.TabIndex = gui.State.Panels.Status.ContextIndex

	switch gui.getStatusContexts()[gui.State.Panels.Status.ContextIndex] {
	case "credits":
		if err := gui.renderCredits(); err != nil {
			return err
		}
	case "logs":
		if err := gui.renderAllLogs(); err != nil {
			return err
		}
	case "config":
		if err := gui.renderDockerComposeConfig(); err != nil {
			return err
		}
	default:
		return errors.New("Unknown context for status panel")
	}

	return nil
}

func (gui *Gui) renderCredits() error {
	return gui.T.NewTask(func(stop chan struct{}) {
		mainView := gui.getMainView()
		mainView.Autoscroll = false
		mainView.Wrap = gui.Config.UserConfig.Gui.WrapMainPanel

		var configBuf bytes.Buffer
		yaml.NewEncoder(&configBuf, yaml.IncludeOmitted).Encode(gui.Config.UserConfig)

		dashboardString := strings.Join(
			[]string{
				lazydockerTitle(),
				"Copyright (c) 2019 Jesse Duffield",
				"Keybindings: https://github.com/jesseduffield/lazydocker/blob/master/docs/keybindings",
				"Config Options: https://github.com/jesseduffield/lazydocker/blob/master/docs/Config.md",
				"Raise an Issue: https://github.com/jesseduffield/lazydocker/issues",
				utils.ColoredString("Buy Jesse a coffee: https://donorbox.org/lazydocker", color.FgMagenta), // caffeine ain't free
				"Here's your lazydocker config when merged in with the defaults (you can open your config by pressing 'o'):",
				configBuf.String(),
			}, "\n\n")

		gui.renderString(gui.g, "main", dashboardString)
	})
}

func (gui *Gui) renderAllLogs() error {
	return gui.T.NewTask(func(stop chan struct{}) {
		mainView := gui.getMainView()
		mainView.Autoscroll = true
		mainView.Wrap = gui.Config.UserConfig.Gui.WrapMainPanel

		gui.clearMainView()

		cmd := gui.OSCommand.RunCustomCommand(
			utils.ApplyTemplate(
				gui.Config.UserConfig.CommandTemplates.AllLogs,
				gui.DockerCommand.NewCommandObject(commands.CommandObject{}),
			),
		)

		cmd.Stdout = mainView
		cmd.Stderr = mainView

		gui.OSCommand.PrepareForChildren(cmd)
		cmd.Start()

		go func() {
			<-stop
			if err := gui.OSCommand.Kill(cmd); err != nil {
				gui.Log.Error(err)
			}
		}()

		cmd.Wait()
	})
}

func (gui *Gui) renderDockerComposeConfig() error {
	return gui.T.NewTask(func(stop chan struct{}) {
		mainView := gui.getMainView()
		mainView.Autoscroll = false
		mainView.Wrap = gui.Config.UserConfig.Gui.WrapMainPanel

		config := gui.DockerCommand.DockerComposeConfig()
		gui.renderString(gui.g, "main", config)
	})
}

func (gui *Gui) handleOpenConfig(g *gocui.Gui, v *gocui.View) error {
	return gui.openFile(gui.Config.ConfigFilename())
}

func (gui *Gui) handleEditConfig(g *gocui.Gui, v *gocui.View) error {
	return gui.editFile(gui.Config.ConfigFilename())
}

func lazydockerTitle() string {
	return `
   _                     _            _
  | |                   | |          | |
  | | __ _ _____   _  __| | ___   ___| | _____ _ __
  | |/ _` + "`" + ` |_  / | | |/ _` + "`" + ` |/ _ \ / __| |/ / _ \ '__|
  | | (_| |/ /| |_| | (_| | (_) | (__|   <  __/ |
  |_|\__,_/___|\__, |\__,_|\___/ \___|_|\_\___|_|
                __/ |
               |___/
`
}

func (gui *Gui) handleStatusNextContext(g *gocui.Gui, v *gocui.View) error {
	contexts := gui.getStatusContexts()
	if gui.State.Panels.Status.ContextIndex >= len(contexts)-1 {
		gui.State.Panels.Status.ContextIndex = 0
	} else {
		gui.State.Panels.Status.ContextIndex++
	}

	gui.handleStatusSelect(gui.g, v)

	return nil
}

func (gui *Gui) handleStatusPrevContext(g *gocui.Gui, v *gocui.View) error {
	contexts := gui.getStatusContexts()
	if gui.State.Panels.Status.ContextIndex <= 0 {
		gui.State.Panels.Status.ContextIndex = len(contexts) - 1
	} else {
		gui.State.Panels.Status.ContextIndex--
	}

	gui.handleStatusSelect(gui.g, v)

	return nil
}

// handleViewAllLogs switches to a subprocess viewing all the logs from docker-compose
func (gui *Gui) handleViewAllLogs(g *gocui.Gui, v *gocui.View) error {
	c, err := gui.DockerCommand.ViewAllLogs()
	if err != nil {
		return gui.createErrorPanel(gui.g, err.Error())
	}

	gui.SubProcess = c
	return gui.Errors.ErrSubProcess
}

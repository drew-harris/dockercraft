package model

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	dtypes "github.com/docker/docker/api/types"
	"github.com/drewharris/shulker/config"
	"github.com/drewharris/shulker/engine"
	"github.com/drewharris/shulker/types"
)

type LoadingModel struct {
	spinner       spinner.Model
	loadingOutput []string
}

type ServerExec struct {
	Connection dtypes.HijackedResponse
}

type Loggers struct {
	error   types.Logger
	build   types.Logger
	server  types.Logger
	startup types.Logger
}

type MainModel struct {
	// TODO: CHANGE VIEW SELECTION TO ENUM
	isLoading          bool
	isShuttingDown     bool
	isBuilding         bool
	isViewingBuildLogs bool
	loadingModel       LoadingModel

	width  int
	height int

	engine engine.Engine
	config config.Config
	keys   KeyMap
	help   help.Model

	outputChan     chan types.OutputMsg
	errorMessages  []string
	serverMessages []string
	buildMessages  []string

	loggers Loggers
}

func (m MainModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, m.loadingModel.spinner.Tick)
	cmds = append(cmds, ListenForOutput(m.outputChan))

	cmds = append(cmds, m.ensureSetupCmd())

	return tea.Batch(cmds...)
}

func (m MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.isShuttingDown = true
			return m, m.Shutdown()

		case key.Matches(msg, m.keys.ToggleBuildLogs):
			m.isViewingBuildLogs = !m.isViewingBuildLogs
			return m, nil

		case key.Matches(msg, m.keys.Attach):
			// Print info in non alt screen
			// return m, tea.ExecProcess(exec.Command("docker", "attach", m.ConatainerId), func(err error) tea.Msg { return nil })
		case key.Matches(msg, m.keys.RebuildAll):
			// Print info in non alt screen
			m.isBuilding = true
			return m, tea.Sequence(m.rebuildAllPlugins(false), func() tea.Msg { return types.DoneBuilding })
		case key.Matches(msg, m.keys.RebuildAllNoCache):
			// Print info in non alt screen
			m.isBuilding = true
			return m, tea.Sequence(m.rebuildAllPlugins(true), func() tea.Msg { return types.DoneBuilding })
		}

	case tea.WindowSizeMsg: // RESIZE
		m.width = msg.Width
		m.height = msg.Height

	// Channel output messages
	case types.OutputMsg:
		switch msg.Target {
		case types.StartupOutput:
			m.loadingModel.loadingOutput = append(m.loadingModel.loadingOutput, msg.Message)
			if len(m.loadingModel.loadingOutput) > m.height/2 {
				m.loadingModel.loadingOutput = m.loadingModel.loadingOutput[1:]
			}
		case types.ErrorOutput:
			m.errorMessages = append(m.errorMessages, msg.Message)
		case types.ServerOutput:
			m.serverMessages = append(m.serverMessages, msg.Message)
		case types.BuildOutput:
			m.buildMessages = append(m.buildMessages, msg.Message)

		}
		return m, ListenForOutput(m.outputChan)

	case types.QuickMsg:
		switch msg {
		case types.DoneBuilding:
			m.isBuilding = false
			return m, nil
		case types.BuildStarted:
			m.isBuilding = true
			return m, nil
		case types.ErrorBuilding:
			m.isBuilding = false
			return m, nil
		case types.FinishedSetup:
			m.isLoading = false
			return m, m.startServerCmd()
		case types.FinishedServerStart:
			return m, nil

		}

	default:
		var cmd tea.Cmd
		m.loadingModel.spinner, cmd = m.loadingModel.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func generateLogFn(sub chan types.OutputMsg, target types.OutputTarget) func(msg string) {
	return func(msg string) {
		sub <- types.OutputMsg{
			Target:  target,
			Message: msg,
		}
	}
}

func InitialModel(engine engine.Engine, config config.Config) MainModel {
	s := spinner.New()
	s.Spinner = spinner.Line

	outputChan := make(chan types.OutputMsg)
	model := MainModel{
		isLoading:  true,
		engine:     engine,
		outputChan: outputChan,
		keys:       DefaultKeyMap,
		help:       help.New(),
		config:     config,
		loadingModel: LoadingModel{
			spinner:       s,
			loadingOutput: []string{},
		},
		loggers: Loggers{
			error:   generateLogFn(outputChan, types.ErrorOutput),
			build:   generateLogFn(outputChan, types.BuildOutput),
			server:  generateLogFn(outputChan, types.ServerOutput),
			startup: generateLogFn(outputChan, types.StartupOutput),
		},
	}

	return model
}

func lastLines(strs []string, amt int) string {
	startIndex := len(strs) - amt
	if startIndex < 0 {
		startIndex = 0
	}

	lastElements := strs[startIndex:]
	return strings.Join(lastElements, "\n")
}

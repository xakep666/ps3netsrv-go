package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/xakep666/ps3netsrv-go/internal/osutil/systemlog"
)

const serviceName = "ps3netsrv-go"

type statusCmd struct{}

func (*statusCmd) Run(s *mgr.Service) error {
	st, err := s.Query()
	if err != nil {
		return err
	}

	state := "<unknown>"
	switch st.State {
	case svc.Stopped:
		state = "stopped"
	case svc.StartPending:
		state = "start pending"
	case svc.Running:
		state = "running"
	case svc.PausePending:
		state = "pause pending"
	case svc.Paused:
		state = "paused"
	case svc.ContinuePending:
		state = "continue pending"
	case svc.StopPending:
		state = "stop pending"
	}

	_, err = fmt.Println("Service state:", state)
	if err != nil {
		return err
	}

	if st.State == svc.Stopped {
		if st.Win32ExitCode != 0 {
			_, err = fmt.Println("Win32 exit code:", int(st.Win32ExitCode))
		}
		if st.ServiceSpecificExitCode != 0 {
			_, err = fmt.Println("Specific exit code:", int(st.ServiceSpecificExitCode))
		}
	}

	return err
}

type installCmd struct {
	LogOnly   bool `help:"Install only system log facility (to use '--system-log') without service"`
	AutoStart bool `help:"Enable auto-start for service"`
}

func (c *installCmd) Run(a *app, m *mgr.Mgr) error {
	err := systemlog.Install()
	switch {
	case errors.Is(err, nil):
		// pass
	case errors.Is(err, fs.ErrExist):
		// already installed, pass
	default:
		return fmt.Errorf("system log install: %w", err)
	}

	if c.LogOnly {
		return nil
	}

	configPath, err := a.AbsoluteConfigPath()
	if err != nil {
		return err
	}

	if configPath == "" {
		return fmt.Errorf("'config' flag is required for service installation")
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get exe path: %w", err)
	}

	startType := uint32(mgr.StartManual)
	if c.AutoStart {
		startType = mgr.StartAutomatic
	}

	_, err = m.CreateService(serviceName, exePath,
		mgr.Config{
			DisplayName: "PS3 Network Server (Go implementation)",
			StartType:   startType,
		},
		"server", "--config="+configPath, "--system-log",
	)
	return err
}

type uninstallCmd struct {
	LogOnly bool `help:"Uninstall only system log facility without service"`
}

func (c *uninstallCmd) Run(m *mgr.Mgr) error {
	err := systemlog.Uninstall()
	switch {
	case errors.Is(err, nil):
		// pass
	case errors.Is(err, fs.ErrNotExist):
		// already uninstalled or not installed, pass
	default:
		return fmt.Errorf("system log install: %w", err)
	}

	if c.LogOnly {
		return nil
	}

	s, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}

	if err := s.Delete(); err != nil {
		return err
	}

	// service won't be deleted until it's stopped
	_, err = s.Control(svc.Stop)
	if errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
		return nil // ignore non-started service
	}
	return err
}

type startCmd struct{}

func (c *startCmd) Run(a *app, s *mgr.Service) error {
	configPath, err := a.AbsoluteConfigPath()
	if err != nil {
		return err
	}

	if configPath != "" {
		return s.Start("server", "--config="+configPath, "--system-log")
	}

	return s.Start()
}

type stopCmd struct{}

func (c *stopCmd) Run(a *app, s *mgr.Service) error {
	_, err := s.Control(svc.Stop)
	return err
}

type updateCmd struct {
	AutoStart *bool `help:"Enable/disable auto-start for service" negatable:""`
}

func (c *updateCmd) Run(a *app, s *mgr.Service) error {
	configPath, err := a.AbsoluteConfigPath()
	if err != nil {
		return err
	}
	if configPath == "" {
		return fmt.Errorf("'config' flag is required for service installation")
	}

	cfg, err := s.Config()
	if err != nil {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get exe path: %w", err)
	}

	// repeats install process
	cfg.BinaryPathName = syscall.EscapeArg(exePath)
	cfg.BinaryPathName += " " + syscall.EscapeArg("server")
	cfg.BinaryPathName += " " + syscall.EscapeArg("--config="+configPath)
	cfg.BinaryPathName += " " + syscall.EscapeArg("--system-log")

	if c.AutoStart != nil {
		if *c.AutoStart {
			cfg.StartType = mgr.StartAutomatic
		} else {
			cfg.StartType = mgr.StartManual
		}
	}

	return s.UpdateConfig(cfg)
}

type svcApp struct {
	Status    statusCmd    `cmd:"status" help:"Show current status of ps3netsrv-go Windows Service"`
	Install   installCmd   `cmd:"install" help:"Install ps3netsrv-go as Windows Service"`
	Uninstall uninstallCmd `cmd:"uninstall" help:"Uninstall ps3netsrv-go Windows Service"`
	Start     startCmd     `cmd:"start" help:"Start ps3netsrv-go Windows Service"`
	Stop      stopCmd      `cmd:"start" help:"Stop ps3netsrv-go Windows Service"`
	Update    updateCmd    `cmd:"update" help:"Update ps3netsrv-go Windows Service"`
}

func (*svcApp) Signature() string {
	return `cmd:"" name:"svc" help:"Control ps3netsrv-go Windows Service"`
}

func (*svcApp) ProvideManager() (*mgr.Mgr, error) {
	return mgr.Connect()
}

func (*svcApp) ProvideService(m *mgr.Mgr) (*mgr.Service, error) {
	s, err := m.OpenService(serviceName)
	if err != nil {
		return nil, fmt.Errorf("open service: %w, perhaps it's not installed", err)
	}
	return s, nil
}

package orchestrator

import (
	"bytes"
	"embed"
	"fmt"
	"github.com/fatih/color"
	"github.com/kardianos/osext"
	"github.com/spf13/cobra"
	"go.amplifyedge.org/booty-v2/internal/downloader"
	"go.amplifyedge.org/booty-v2/internal/gitutil"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"go.amplifyedge.org/booty-v2/cmd"
	"go.amplifyedge.org/booty-v2/config"
	"go.amplifyedge.org/booty-v2/dep"
	rg "go.amplifyedge.org/booty-v2/dep/registry"
	"go.amplifyedge.org/booty-v2/internal/errutil"
	"go.amplifyedge.org/booty-v2/internal/logging"
	"go.amplifyedge.org/booty-v2/internal/logging/zaplog"
	"go.amplifyedge.org/booty-v2/internal/osutil"
	"go.amplifyedge.org/booty-v2/internal/store"
	"go.amplifyedge.org/booty-v2/internal/store/file"
	"go.amplifyedge.org/booty-v2/internal/update"
	sharedCmd "go.amplifyedge.org/shared-v2/tool/bs-crypt/cmd"
	langCmd "go.amplifyedge.org/shared-v2/tool/bs-lang/cmd"
)

//go:embed makefiles/*
var makefilesFs embed.FS

const binName = "booty"

// Orchestrator implements Executor, Agent, and Commander
type Orchestrator struct {
	cfg           *config.AppConfig
	components    map[string]dep.Component
	logger        logging.Logger
	command       *cobra.Command
	db            store.Storer
	gw            dep.GitWrapper
	buildVersion  update.Version
	buildRevision string
}

// constructor
func NewOrchestrator(app string, buildVersion, buildRevision string) *Orchestrator {
	var ac *config.AppConfig
	var comps map[string]dep.Component
	var rootCmd = &cobra.Command{
		Use:     app,
		Version: fmt.Sprintf("%s revision: %s", buildVersion, buildRevision),
	}
	// setup logger
	logger := zaplog.NewZapLogger(zaplog.INFO, app, true)
	logger.InitLogger(nil)
	// config, loads default config if it doesn't exists
	etc := osutil.GetEtcDir()
	configPath := filepath.Join(etc, "config.json")
	fileContent, err := ioutil.ReadFile(configPath)
	if err != nil {
		// use default config
		fileContent, err = config.DefaultConfig.ReadFile("config.reference.json")
		if err != nil {
			logger.Fatalf("error encoding default json config")
		}
		// write it to default path
		err = ioutil.WriteFile(configPath, fileContent, 0644)
		if err != nil {
			logger.Fatalf("error writing default json config")
		}
	}
	ac = config.NewAppConfig(logger, fileContent)

	// setup file database for package tracking
	db, err := file.NewDB(logger, filepath.Join(osutil.GetDataDir(), "packages"), false)
	if err != nil {
		logger.Fatalf("error creating database: %v", err)
	}

	exPath, err := osext.Executable()
	if err != nil {
		logger.Fatalf("unable to get path to executable: %v", err)
	}

	err = db.New(&store.InstalledPackage{
		Name:    binName,
		Version: buildVersion,
		FilesMap: map[string]string{
			exPath: buildRevision,
		},
	})

	if err != nil {
		logger.Fatalf("error populating initial package: %v", err)
	}

	repoDb, err := file.NewDB(logger, filepath.Join(osutil.GetDataDir(), "repos"), true)
	if err != nil {
		logger.Fatalf("error creating repo database: %v", err)
	}
	gw := gitutil.NewHelper(repoDb, ac.GitEmail)

	// setup registry
	registry, err := rg.NewRegistry(db, buildVersion)
	if err != nil {
		logger.Fatalf("error creating components: %v", err)
	}

	if ac.DevMode {
		comps = registry.DevComponents
	} else {
		comps = registry.Components
	}

	return &Orchestrator{
		cfg:           ac,
		components:    comps,
		logger:        logger,
		command:       rootCmd,
		db:            db,
		gw:            gw,
		buildVersion:  update.Version(buildVersion),
		buildRevision: buildRevision,
	}
}

// =================================================================
// Commander
// =================================================================
func (o *Orchestrator) Command() *cobra.Command {
	extraCmds := []*cobra.Command{
		cmd.InstallAllCommand(o),
		cmd.InstallCommand(o),
		cmd.UninstallAllCommand(o),
		cmd.UninstallCommand(o),
		cmd.UpdateAllCommand(o, o),
		cmd.AgentCommand(o, o),
		cmd.RunCommand(o),
		cmd.RunAllCommand(o),
		cmd.StopAllCommand(o),
		cmd.StopCommand(o),
		cmd.ListAllCommand(o),
		// here we exported all the internal tools we might need (bs-crypt, bs-lang, etc)
		sharedCmd.EncryptCmd(),
		sharedCmd.DecryptCmd(),
		langCmd.RootCmd,
		cmd.GitWrapperCmd(o.gw),
		cmd.OsPrintCommand(o),
		cmd.CleanCacheCmd(o),
		cmd.CompletionCommand(o),
		cmd.HugoCommand(o),
		cmd.PkgRepoServerCmd(),
		cmd.PkgRepoClientCmd(),
	}
	if o.cfg.DevMode {
		extraCmds = append(
			extraCmds,
			cmd.ProtoCommand(o),
			cmd.ReleaseCommand(o),
			cmd.JbCommand(o),
			cmd.JsonnetCommand(o),
			cmd.ExtractCommand(o),
			cmd.CertCommand(o),
			cmd.CopyCertCommand(o),
			cmd.HoverCommand(o),
		)
	}
	o.command.AddCommand(extraCmds...)
	o.command.TraverseChildren = true
	return o.command
}

func (o *Orchestrator) Completion() ([]byte, error) {
	shell, err := o.getShell()
	if err != nil {
		return nil, err
	}
	buf := bytes.Buffer{}
	o.logger.Info("user shell is: " + shell)
	switch shell {
	case "powershell":
		if err = o.command.GenPowerShellCompletion(&buf); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "zsh":
		if err = o.command.GenZshCompletion(&buf); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "bash":
		if err = o.command.GenBashCompletion(&buf); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "fish":
		if err = o.command.GenFishCompletion(&buf, true); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	default:
		return nil, errutil.New(errutil.ErrUnsupportedShell, fmt.Errorf("%s", shell))
	}
}

func (o *Orchestrator) getShell() (string, error) {
	var shellName string
	switch osutil.GetOS() {
	case "darwin":
		out, err := exec.Command("dscl", "localhost", "-read", "/Local/Default/Users/"+os.Getenv("USER"), "UserShell").Output()
		if err != nil {
			return "", err
		}
		splitted := strings.TrimSpace(strings.Split(string(out), ":")[1])
		shellName = filepath.Base(splitted)
	case "linux":
		u, err := user.Current()
		if err != nil {
			return "", err
		}

		out, err := exec.Command("getent", "passwd", u.Uid).Output()
		if err != nil {
			return "", err
		}

		shell := strings.Split(strings.TrimSuffix(string(out), "\n"), ":")
		return filepath.Base(shell[6]), nil
	case "windows":
		shellName = "powershell"
	}
	return shellName, nil
}

func (o *Orchestrator) Logger() logging.Logger {
	return o.logger
}

// =================================================================
// Executor
// =================================================================

func (o *Orchestrator) Component(name string) dep.Component {
	o.logger.Debugf("querying component of name: %s", name)
	if o.components[name] != nil {
		return o.components[name]
	}
	return nil
}

func (o *Orchestrator) AllComponents() []dep.Component {
	var comps []dep.Component
	for _, v := range o.components {
		comps = append(comps, v)
	}
	return comps
}

func (o *Orchestrator) DownloadAll() error {
	o.logger.Info("downloading all components")
	if err := o.setupVersions(); err != nil {
		return err
	}
	var tasks []*task
	for _, c := range o.components {
		k := c
		if k.Name() != binName {
			tasks = append(tasks, newTask(k.Download, dlErr(k)))
		}
	}
	pool := newTaskPool(tasks)
	pool.runAll()
	for _, t := range pool.tasks {
		if t.err != nil {
			return t.errFunc(t.err)
		}
	}
	return nil
}

func (o *Orchestrator) setupVersions() error {
	var tasks []*task
	for _, c := range o.components {
		k := c
		if k.Dependencies() != nil {
			for _, d := range k.Dependencies() {
				tasks = append(tasks, newTask(setVersion(o.cfg, d), dlErr(d)))
			}
		}
		if k.Name() == binName {
			k.SetVersion(o.buildVersion)
		} else {
			tasks = append(tasks, newTask(setVersion(o.cfg, k), dlErr(k)))
		}
	}
	pool := newTaskPool(tasks)
	pool.runAll()
	for _, t := range pool.tasks {
		if t.err != nil {
			return t.errFunc(t.err)
		}
	}
	return nil
}

func dlErr(c dep.Component) func(err error) error {
	return func(err error) error {
		return errutil.New(errutil.ErrDownloadComponent, fmt.Errorf("name: %s, version: %s, error: %v", c.Name(), c.Version(), err))
	}
}

func (o *Orchestrator) Run(name string, args ...string) error {
	comp := o.Component(name)
	if err := comp.Run(args...); err != nil {
		return fmt.Errorf(" err: %v", err)
	}
	return nil
}

func (o *Orchestrator) Stop(name string) error {
	c := o.Component(name)
	if c == nil {
		return nil
	}
	return c.RunStop()
}

func (o *Orchestrator) StopAll() error {
	for _, c := range o.components {
		if c.IsService() {
			if err := c.RunStop(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (o *Orchestrator) Install(name, version string) error {
	var err error
	c := o.Component(name)
	if c == nil {
		err = errutil.New(errutil.ErrInvalidComponent, fmt.Errorf("name: %s, version: %s", name, version))
		return err
	}
	// download it
	c.SetVersion(update.Version(version))
	if c.Dependencies() != nil {
		for _, d := range c.Dependencies() {
			if err = setVersion(o.cfg, d)(); err != nil {
				return err
			}
			if err = d.Download(); err != nil {
				return err
			}
			if err = d.Install(); err != nil {
				return err
			}
		}
	}
	if err = c.Download(); err != nil {
		return errutil.New(errutil.ErrDownloadComponent, fmt.Errorf("name: %s, version: %s, err: %v", c.Name(), c.Version(), err))
	}
	// try installing it
	if err = c.Install(); err != nil {
		return errutil.New(errutil.ErrInstallComponent, fmt.Errorf("name: %s, version: %s, err: %v", name, version, err))
	}
	return nil
}

func (o *Orchestrator) InstallAll() error {
	// we don't run concurrently here.
	o.logger.Info("installing all components")
	for _, c := range o.components {
		k := c
		if k.Name() != binName {
			o.logger.Infof("installing %s, version: %s", k.Name(), k.Version())
			if err := k.Install(); err != nil {
				return errutil.New(errutil.ErrInstallComponent, fmt.Errorf("name: %s, version: %s, err: %v", k.Name(), k.Version(), err))
			}
		}
	}
	o.logger.Info("Installed successfully")
	return nil
}

func (o *Orchestrator) Uninstall(name string) error {
	var err error
	o.logger.Infof("uninstall %s", name)
	c := o.Component(name)
	if c == nil {
		return errutil.New(errutil.ErrUninstallComponent, fmt.Errorf("name: %s, err: no package of that name available", name))
	}
	if c.Name() != binName {
		o.logger.Infof("uninstalling %s, version: %s", c.Name(), c.Version())
		if err = c.Uninstall(); err != nil {
			return errutil.New(errutil.ErrUninstallComponent, fmt.Errorf("name: %s, version: %s, err: %v", c.Name(), c.Version(), err))
		}
	}
	return nil
}

func (o *Orchestrator) UninstallAll() error {
	o.logger.Info("uninstall all components")
	// Best effort uninstall everything first. Not everything may be installed.
	// That can happen if an install failed or uninstall-all was interrupted.
	for _, c := range o.components {
		if c.Name() != binName {
			if err := c.Uninstall(); err != nil {
				o.logger.Infof("uninstalling %s, version: %s", c.Name(), c.Version())
				o.logger.Error(errutil.New(errutil.ErrUninstallComponent, fmt.Errorf("name: %s, version: %s, err: %v", c.Name(), c.Version(), err)))
			}
		}
	}
	return nil
}

func (o *Orchestrator) Backup(name string) error {
	c := o.Component(name)
	if c != nil {
		return c.Backup()
	}
	return nil
}

func (o *Orchestrator) BackupAll() error {
	for _, c := range o.components {
		if err := c.Backup(); err != nil {
			return err
		}
	}
	return nil
}

func (o *Orchestrator) AllInstalledComponents() ([]byte, error) {
	// TODO
	pkgs, err := o.db.List()
	if err != nil {
		return nil, err
	}
	var b []byte
	buf := bytes.NewBuffer(b)
	tw := tabwriter.NewWriter(buf, 20, 2, 4, ' ', tabwriter.TabIndent)
	_, _ = fmt.Fprintf(tw, "%s", headerColor("Installed Packages:"))
	for _, p := range pkgs {
		printRow(tw, "\n%s\t\t%s", p.Name, p.Version)
	}
	printRow(tw, "\n%s%s", "", "")
	err = tw.Flush()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var (
	headerColor  = color.New(color.FgCyan).SprintFunc()
	pkgColor     = color.New(color.FgHiGreen).SprintFunc()
	versionColor = color.New(color.FgYellow).SprintFunc()
)

func printRow(out io.Writer, format, key string, value string) {
	_, _ = fmt.Fprintf(out, format, pkgColor(key), versionColor(value))
}

func (o *Orchestrator) RunAll() error {
	for _, c := range o.components {
		if c.IsService() {
			o.logger.Infof("running %s", c.Name())
			if err := c.Run(); err != nil {
				return fmt.Errorf("%s: %v", c.Name(), err)
			}
		}
	}
	return nil
}

func (o *Orchestrator) CleanCache() error {
	cacheDir := osutil.GetDownloadDir()
	if empty, _ := downloader.IsEmptyDir(cacheDir); empty {
		return nil
	}
	entries, err := ioutil.ReadDir(cacheDir)
	if err != nil {
		return errutil.New(errutil.ErrCleaningCache, fmt.Errorf(": %v", err))
	}
	for _, e := range entries {
		if err = os.RemoveAll(filepath.Join(cacheDir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// ================================================================
// Agent
// ================================================================

func (o *Orchestrator) Checker() *update.Checker {
	var err error

	// create new checker instance
	repos := map[update.RepositoryURL]update.Version{}
	for _, c := range o.components {
		repos[c.RepoUrl()] = c.Version()
	}
	checker := update.NewChecker(o.logger, repos, func(r update.RepositoryURL, v update.Version) error {
		// find component matching the repo url
		for _, c := range o.components {
			if c.RepoUrl() == r && v != c.Version() {
				if c.IsService() {
					if err = c.RunStop(); err != nil {
						return errutil.New(errutil.ErrUpdateComponent, fmt.Errorf("stopping component service error, %s version %s error: %v", c.Name(), c.Version(), err))
					}
				}
				if err = c.Update(v); err != nil {
					return errutil.New(errutil.ErrUpdateComponent, fmt.Errorf("%s version %s error: %v", c.Name(), c.Version(), err))
				}
			}
		}
		return nil
	})
	return checker
}

func setVersion(ac *config.AppConfig, c dep.Component) func() error {
	var err error
	return func() error {
		v := ac.GetVersion(c.Name())
		if v == "" {
			v, err = update.GetLatestVersion(c.RepoUrl())
			if err != nil {
				return err
			}
		}
		c.SetVersion(update.Version(v))
		return nil
	}
}

// ================================================================
// OSInfo
// ================================================================

func (o *Orchestrator) OSInfo() string {
	return osutil.GetOSInfo()
}

// ================================================================
// Extractor
// ================================================================
func (o *Orchestrator) Extract(dirpath string) error {
	dirExists := osutil.DirExists(dirpath)
	if !dirExists {
		if err := os.MkdirAll(dirpath, 0755); err != nil {
			return err
		}
	}
	files, err := makefilesFs.ReadDir("makefiles")
	if err != nil {
		return err
	}
	for _, f := range files {
		srcPath := filepath.Join("makefiles", f.Name())
		destPath := filepath.Join(dirpath, f.Name())
		srcContent, err := makefilesFs.ReadFile(srcPath)
		if err != nil {
			return err
		}
		dst, err := os.Create(destPath)
		if err != nil {
			return err
		}
		src := bytes.NewBuffer(srcContent)
		_, err = io.Copy(dst, src)
		if err != nil {
			return err
		}
	}
	return nil
}

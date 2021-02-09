package components

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.amplifyedge.org/booty-v2/pkg/downloader"
	"go.amplifyedge.org/booty-v2/pkg/fileutil"

	// "path/filepath"

	"go.amplifyedge.org/booty-v2/pkg/osutil"
	"go.amplifyedge.org/booty-v2/pkg/store"
)

const (
	// version -- os-arch
	goreleaserUrlFormat = "https://github.com/goreleaser/goreleaser/releases/download/v%s/goreleaser_%s.%s"
)

type Goreleaser struct {
	version string
	dlPath  string
	db      *store.DB
}

func NewGoreleaser(db *store.DB, version string) *Goreleaser {
	return &Goreleaser{
		version: version,
		dlPath:  "",
		db:      db,
	}
}

func (g *Goreleaser) Version() string {
	return g.version
}

func (g *Goreleaser) Name() string {
	return "goreleaser"
}

func (g *Goreleaser) Download(targetDir string) error {
	downloadDir := filepath.Join(targetDir, g.Name()+"-"+g.version)
	_ = os.MkdirAll(downloadDir, 0755)
	osname := fmt.Sprintf("%s_%s", osutil.GetOS(), osutil.GetAltArch())
	var ext string
	switch osutil.GetOS() {
	case "linux", "darwin":
		ext = "tar.gz"
	case "windows":
		ext = "zip"
	}
	fetchUrl := fmt.Sprintf(goreleaserUrlFormat, g.version, osname, ext)
	err := downloader.Download(fetchUrl, downloadDir)
	if err != nil {
		return err
	}
	g.dlPath = downloadDir
	return nil
}

func (g *Goreleaser) Install() error {
	var err error
	// install to global path
	// create bin directory under $PREFIX
	binDir := osutil.GetBinDir()
	// all files that are going to be installed
	executableName := g.Name()
	switch osutil.GetOS() {
	case "windows":
		executableName += ".exe"
	}
	filesMap := map[string][]interface{}{
		filepath.Join(g.dlPath, executableName): {filepath.Join(binDir, executableName), 0755},
	}
	ip := store.InstalledPackage{
		Name:     g.Name(),
		Version:  g.version,
		FilesMap: map[string]int{},
	}
	// copy file to the global bin directory
	for k, v := range filesMap {
		if err = fileutil.Copy(k, v[0].(string)); err != nil {
			return err
		}
		installedName := v[0].(string)
		installedMode := v[1].(int)
		if err = os.Chmod(installedName, os.FileMode(installedMode)); err != nil {
			return err
		}
		ip.FilesMap[installedName] = installedMode
	}
	if err = g.db.New(&ip); err != nil {
		return err
	}
	return os.RemoveAll(g.dlPath)
}

func (g *Goreleaser) Uninstall() error {
	var err error
	// install to global path
	// all files that are going to be installed
	var pkg *store.InstalledPackage
	pkg, err = g.db.Get(g.Name())
	if err != nil {
		return err
	}
	var filesList []string
	for k := range pkg.FilesMap {
		filesList = append(filesList, k)
	}
	// uninstall listed files
	for _, file := range filesList {
		if err = os.RemoveAll(file); err != nil {
			return err
		}
	}
	// remove downloaded files
	return os.RemoveAll(g.dlPath)
}

func (g *Goreleaser) Update(version string) error {
	g.version = version
	targetDir := filepath.Dir(g.dlPath)
	if err := g.Uninstall(); err != nil {
		return err
	}
	if err := g.Download(targetDir); err != nil {
		return err
	}
	return g.Install()
}

func (g *Goreleaser) Run(args ...string) error {
	pkg, err := g.db.Get(g.Name())
	if err != nil {
		return err
	}
	for k, _ := range pkg.FilesMap {
		if strings.Contains(k, g.Name()) {
			return osutil.Exec(k, args...)
		}
	}
	return nil
}

func (g *Goreleaser) Backup() error {
	// We don't need to implement this
	return nil
}

func (g *Goreleaser) RunStop() error {
	// We don't need to implement this
	return nil
}
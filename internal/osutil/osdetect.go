package osutil

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// global variables
// yes.
var (
	dirs  appDirs
	rinfo runtimeInfo
	err   error
)

func init() {
	rinfo = setupRuntimeInfo()
	dirs, err = setupDirs()
	if err != nil {
		log.Fatal(err)
	}
}

func setupDirs() (appDirs, error) {
	prefix := getInstallPrefix()
	dirs := []string{"bin", "etc", "data", "cache", "include", "uploads"}
	ad := appDirs{}
	for i := range dirs {
		dirPath := filepath.Join(prefix, dirs[i])
		if err = os.MkdirAll(dirPath, 0755); err != nil {
			return ad, err
		}
		switch dirs[i] {
		case "bin":
			ad.bin = dirPath
		case "etc":
			ad.etc = dirPath
		case "data":
			ad.data = dirPath
		case "cache":
			ad.cache = dirPath
		case "include":
			ad.include = dirPath
		case "uploads":
			ad.include = dirPath
		}
	}
	return ad, nil
}

// appDirs containing directories this app is using
type appDirs struct {
	bin     string // contains binaries
	data    string // contains database
	etc     string // contains configurations
	include string // contains shared library
	cache   string // contains downloaded tarballs
	uploads string // contains uploaded binaries
}

// runtimeInfo for the whole application
type runtimeInfo struct {
	osName  string // os name
	arch    string // arch name
	altOs   string // alt os name
	altArch string // alt architecture
}

func (ri runtimeInfo) String() string {
	return fmt.Sprintf(`
	--------- Basic Info ----------------
	OS: %s
	Arch: %s

	--------- Booty Info ----------------
	Prefix: %s

	-------------------------------------
`, ri.osName, ri.arch, getInstallPrefix())
}

func GetOSInfo() string {
	return rinfo.String()
}

func setupRuntimeInfo() runtimeInfo {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	var altOs string
	var altArch string
	switch osName {
	case "linux":
		altOs = "linux"
	case "darwin":
		altOs = "mac"
	case "windows":
		altOs = "win"
	default:
		altOs = ""
	}
	switch arch {
	case "amd64":
		altArch = "x86_64"
	case "arm64":
		altArch = "arm64v8"
	default:
		altArch = ""
	}
	return runtimeInfo{
		osName:  osName,
		arch:    arch,
		altOs:   altOs,
		altArch: altArch,
	}
}

func GetOS() string {
	return rinfo.osName
}

func GetAltOs() string {
	return rinfo.altOs
}

func GetArch() string {
	return rinfo.arch
}

// We support only x86_64 or arm64 only for now
func GetAltArch() string {
	return rinfo.altArch
}

func getInstallPrefix() string {
	bh := os.Getenv("BOOTY_HOME")
	if bh != "" {
		return bh
	}
	u, _ := user.Current()
	switch strings.ToLower(GetOS()) {
	case "windows":
		return filepath.Join(`C:\\ProgramData`, "booty")
	case "linux":
		return fmt.Sprintf("%s/.local/%s", u.HomeDir, "booty")
	case "darwin":
		return fmt.Sprintf("%s/Library/Application Support/%s", u.HomeDir, "booty")
	default:
		return "/usr/local"
	}
}

func GetBinDir() string {
	return dirs.bin
}

func GetGoPath() string {
	gopath := os.Getenv("GOPATH")
	if len(gopath) == 0 {
		// Default GOPATH is $HOME/go if the env isn't set.
		if dirname, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(dirname, "go")
		}
	}
	return gopath
}

func GetEtcDir() string {
	return dirs.etc
}

func GetDataDir() string {
	return dirs.data
}

func GetDownloadDir() string {
	return dirs.cache
}

func GetUploadDir() string {
	return dirs.uploads
}

func GetIncludeDir() string {
	return dirs.include
}

func CurUserChown(dir string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		return err
	}
	c := exec.Command("chown", fmt.Sprintf("%s:%s", u.Name, g.Name), dir)
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	return c.Run()
}

// ExecSudo executes a command under "sudo".
func ExecSudo(cmd string, args ...string) error {
	scmd := exec.Command("sudo", append([]string{cmd}, args...)...)
	scmd.Stdin = os.Stdin
	scmd.Stderr = os.Stderr
	scmd.Stdout = os.Stdout

	return scmd.Run()
}

// Exec
func Exec(cmd string, args ...string) error {
	scmd := exec.Command(cmd, args...)
	scmd.Stdin = os.Stdin
	scmd.Stderr = os.Stderr
	scmd.Stdout = os.Stdout

	return scmd.Run()
}

// DetectPreq detect prequisite dependencies (golang and flutter)
// if it doesn't exists in the system, then return error
func DetectPreq() error {
	var executableName = "go"
	if GetOS() == "windows" {
		executableName += ".exe"
	}
	golangExists := ExeExists(executableName)
	gopath := GetGoPath()
	if !golangExists {
		return fmt.Errorf("golang executable doesn't exists, please install golang first")
	}
	if len(gopath) == 0 {
		return fmt.Errorf("GOPATH env is not set. Must set GOPATH.")
	}
	if !DirExists(gopath) {
		return fmt.Errorf("GOPATH dir %s does not exist.", gopath)
	}
	return nil
}

func ExeExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func Exists(fpath string) bool {
	_, err = os.Stat(fpath)
	if err != nil {
		return false
	}
	return true
}

func DirExists(dpath string) bool {
	d, err := os.Stat(dpath)
	if err != nil || !d.IsDir() {
		return false
	}
	return true
}

func checkEnv(key string) bool {
	env := os.Getenv(key)
	return env != ""
}

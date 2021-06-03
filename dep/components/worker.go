package components

import (
	"fmt"
	"go.amplifyedge.org/booty-v2/dep"
	"go.amplifyedge.org/booty-v2/internal/fileutil"
	"go.amplifyedge.org/booty-v2/internal/osutil"
	"go.amplifyedge.org/booty-v2/internal/store"
	"go.amplifyedge.org/booty-v2/internal/update"
	"os"
	"path/filepath"
)

func getDlPath(name, version string) string {
	return filepath.Join(osutil.GetDownloadDir(), name+"-"+version)
}

func commonInstall(c dep.Component, filesMap map[string][]interface{}) (*store.InstalledPackage, error) {
	ip := store.InstalledPackage{
		Name:     c.Name(),
		Version:  c.Version().String(),
		FilesMap: map[string]string{},
	}
	for k, v := range filesMap {
		sum, err := fileutil.Copy(k, v[0].(string))
		if err != nil {
			return nil, fmt.Errorf("Copying %s to %s, err: %v", k, v[0].(string), err)
		}
		installedName := v[0].(string)
		installedMode := v[1].(int)
		if err = os.Chmod(installedName, os.FileMode(installedMode)); err != nil {
			return nil, err
		}
		ip.FilesMap[installedName] = sum
	}
	return &ip, nil
}

func commonUpdate(c dep.Component, version update.Version) error {
	var err error
	c.SetVersion(version)
	if err = c.RunStop(); err != nil {
		return err
	}
	if err = c.Uninstall(); err != nil {
		return err
	}
	if err = c.Download(); err != nil {
		return err
	}
	return c.Install()
}

func commonGetVersion(c dep.Component, db store.Storer) *update.Version {
	pkg, err := db.Get(c.Name())
	if err != nil {
		return nil
	}
	if pkg != nil {
		v := update.Version(pkg.Version)
		return &v
	}
	return nil
}

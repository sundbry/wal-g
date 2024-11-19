package binary

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"

	"github.com/pkg/errors"
	"github.com/wal-g/tracelog"
	conf "github.com/wal-g/wal-g/internal/config"
)

const (
	mongoFsLock = "mongod.lock"
)

type LocalStorage struct {
	MongodDBPath string
	whitelist    *regexp.Regexp
}

func CreateLocalStorage(mongodDBPath string) *LocalStorage {
	return &LocalStorage{
		MongodDBPath: mongodDBPath,
		whitelist:    CreateWhiteList(mongodDBPath),
	}
}

func (localStorage *LocalStorage) EnsureMongodFsLockFileIsEmpty() error {
	mongoFsLockFilePath := path.Join(localStorage.MongodDBPath, mongoFsLock)

	tracelog.InfoLogger.Printf("Check mongod has been shutdown (file '%v' should be empty)", mongoFsLockFilePath)

	lockFileStat, err := os.Stat(mongoFsLockFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			tracelog.WarningLogger.Printf("Mongod lock file '%v' not exists. "+
				"May it remove by previous restore process or manually?", mongoFsLockFilePath)
			return nil
		}
		return errors.Wrapf(err, "check for lock file %s", mongoFsLockFilePath)
	}

	if lockFileStat.Size() != 0 {
		return fmt.Errorf("mongod lock file %s, so it can be run or incorrectly turned off", mongoFsLock)
	}
	return nil
}

func (localStorage *LocalStorage) CleanupMongodDBPath() error {
	tracelog.InfoLogger.Printf("Cleanup data in dbPath '%v'", localStorage.MongodDBPath)

	openedDBPath, err := os.Open(localStorage.MongodDBPath)
	if err != nil {
		return errors.Wrap(err, "open dir")
	}
	defer func() { _ = openedDBPath.Close() }()

	names, err := openedDBPath.Readdirnames(-1)
	if err != nil {
		return errors.Wrap(err, "read file names")
	}
	if len(names) == 0 {
		tracelog.WarningLogger.Printf("dbPath '%v' is empty already", localStorage.MongodDBPath)
		return nil
	}
	for _, name := range names {

		fullPath := filepath.Join(localStorage.MongodDBPath, name)
		if localStorage.whitelist != nil && localStorage.whitelist.MatchString(fullPath) {
			tracelog.InfoLogger.Printf("skip remove %s", filepath.Join(localStorage.MongodDBPath, name))
			continue
		}

		err = os.RemoveAll(fullPath)
		if err != nil {
			return errors.Wrapf(err, "unable to remove '%s' in '%s'", name, localStorage.MongodDBPath)
		}
		tracelog.InfoLogger.Printf("remove %s", filepath.Join(localStorage.MongodDBPath, name))
	}
	return nil
}

func (localStorage *LocalStorage) EnsureEmptyDBPath() error {
	openedPath, err := os.Open(localStorage.MongodDBPath)
	if err != nil {
		return errors.Wrap(err, "open dir")
	}
	defer func() { _ = openedPath.Close() }()

	names, err := openedPath.Readdirnames(-1)
	if err == io.EOF {
		return nil
	}

	if err != nil {
		return errors.Wrap(err, "readdirnames dir")
	}

	for _, name := range names {
		if localStorage.whitelist == nil || !localStorage.whitelist.MatchString(name) {
			return fmt.Errorf("directory '%v' is not empty", localStorage.MongodDBPath)
		}
	}

	return nil
}

func CreateWhiteList(dirPath string) *regexp.Regexp {
	filesRegexp, ok := conf.GetSetting(conf.MongoDBDeletionProtectionWhitelist)
	_, err := regexp.Compile(filesRegexp)

	if !ok || err != nil {
		filesRegexp = `lost\+found`
	}

	return regexp.MustCompile(filepath.Join(regexp.QuoteMeta(dirPath), filesRegexp))
}

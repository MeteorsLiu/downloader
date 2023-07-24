package download

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/boltdb/bolt"
)

var (
	globalDB = NewRecoverable()
)

type Recoverable struct {
	db *bolt.DB
}

func getUserPath() string {
	udr, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	udrName := filepath.Join(udr, "MeteorDownloader")
	if err := os.MkdirAll(udrName, 0755); err != nil {
		return ""
	}
	return udrName
}

func getTempPath() string {
	udr, err := os.UserConfigDir()
	if err != nil {
		os.MkdirAll("recoverTemp", 0755)
		return "recoverTemp"
	}
	udrName := filepath.Join(udr, "MeteorDownloader", "recoverTemp")
	if err := os.MkdirAll(udrName, 0755); err != nil {
		os.MkdirAll("recoverTemp", 0755)
		return "recoverTemp"
	}
	return udrName
}

func NewRecoverable() *Recoverable {
	db, err := bolt.Open(filepath.Join(getUserPath(), "unfinished.db"), 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		printMsg("无法创建断点重传数据库或断点重传数据库已损坏")
		return nil
	}
	runtime.SetFinalizer(db, func(d *bolt.DB) {
		// make sure unlock the database file.
		d.Close()
	})
	return &Recoverable{db: db}
}

func (r *Recoverable) Close() {
	r.db.Close()
}
func (r *Recoverable) Save(id string, js []byte) error {
	return r.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(id))
		if err != nil {
			return err
		}

		return b.Put([]byte("js"), js)
	})
}

func (r *Recoverable) Recover() [][]byte {
	jss := [][]byte{}
	r.db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(_ []byte, b *bolt.Bucket) error {
			jss = append(jss, b.Get([]byte("js")))
			return nil
		})
	})
	return jss
}

func (r *Recoverable) Finished(name string) error {
	return r.db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket([]byte(name))
	})
}

func Save(id string, js []byte) error {
	if globalDB != nil {
		return globalDB.Save(id, js)
	}
	return nil
}

func Recover() [][]byte {
	if globalDB != nil {
		return globalDB.Recover()
	}
	return nil
}

func Finished(name string) error {
	if globalDB != nil {
		return globalDB.Finished(name)
	}
	return nil
}

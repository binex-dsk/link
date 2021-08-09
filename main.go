// A URL Shortener called Link.
// Copyright (C) 2021 i@fsh.ee
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
package main

import (
	"crypto/md5"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"hash/maphash"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//go:embed index.html
var indexTemplate string

type Retry struct {
	retryAttemptCount int
}

func NewRetry(retryAttemptCount int) (Retry, error) {
	if retryAttemptCount < 1 {
		return Retry{}, errors.New("retry attempt count must be greater than zero")
	}
	return Retry{retryAttemptCount}, nil
}

func (r Retry) Do(f func() error) (err error) {
	for i := 0; i < r.retryAttemptCount; i++ {
		err = f()
		if err == nil {
			return nil
		}

	}
	return err
}

type DB struct {
	*gorm.DB
	log      *log.Logger
	hashSeed string
	retry    Retry
}

func NewDB(l *log.Logger, dbFilePath, hashSeed string, retry Retry) (DB, error) {
	_, err := os.Stat(dbFilePath)
	if os.IsNotExist(err) {
		err := ioutil.WriteFile(dbFilePath, []byte{}, 0600)
		if err != nil {
			return DB{}, err
		}
	}
	db, err := gorm.Open(sqlite.Open(dbFilePath), &gorm.Config{
		NowFunc: func() time.Time { return time.Now().UTC() },
		Logger:  logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return DB{}, err
	}
	return DB{db, l, hashSeed, retry}, db.AutoMigrate(&Link{})
}

type Link struct {
	gorm.Model
	Big  string
	Smol string `gorm:"unique"`
	Del  string `gorm:"unique"`
}

func (db DB) getHashShortLink(s fmt.Stringer) (string, error) {
	var (
		h      = maphash.Hash{}
		_, err = h.WriteString(s.String())
	)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.TrimLeft(fmt.Sprintf("%#x\n", h.Sum64()), "0x")), nil
}

func (db DB) getHashDeleteKey(s fmt.Stringer) string {
	return strings.TrimSpace(fmt.Sprintf("%x", md5.Sum([]byte(db.hashSeed+s.String()+strconv.FormatInt(time.Now().Unix(), 10)))))
}

func (db DB) NewLink(u *url.URL) (Link, error) {
	h, err := db.getHashShortLink(u)
	if err != nil {
		return Link{}, err
	}
	return db.NewLinkWithShortLink(u, h)
}

func (db DB) NewLinkWithShortLink(u *url.URL, hash string) (link Link, err error) {
	// Retry for unique errors.
	err = db.retry.Do(func() error {
		link = Link{Big: u.String(), Smol: hash, Del: db.getHashDeleteKey(u)}
		return db.Create(&link).Error
	})
	return
}

func (db DB) GetLink(smol string) (l Link, e error) {
	res := db.Where(&Link{Smol: smol}).First(&l)
	return l, res.Error
}

func (db DB) DelLink(smol, del string) error {
	link, err := db.GetLink(smol)
	if err != nil {
		return err
	}
	res := db.Where(&Link{Del: del}).Delete(&link)
	if res.RowsAffected < 1 {
		return gorm.ErrRecordNotFound
	}
	return res.Error
}

type controller struct {
	log       *log.Logger
	db        DB
	demo      bool
	url, copy string
	tmpl      *template.Template
}

func NewController(logger *log.Logger, db DB, demo bool, url, copy string, tmpl *template.Template) controller {
	return controller{logger, db, demo, strings.TrimRight(url, "/"), copy, tmpl}
}

func (c controller) Err(rw http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		rw.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(rw, "%s", err)
		return
	}
	c.log.Println(err)
	rw.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(rw, "%s", err)
}

func (c controller) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	switch r.Method {

	case http.MethodGet:
        st := strings.TrimRight(r.URL.Path, "/")
        rq, err := url.QueryUnescape(r.URL.RawQuery)
        if err != nil {
            c.Err(rw, r, err)
            return
        }
        if rq != "" {
            u, err := url.Parse(rq)
            if err != nil {
                c.Err(rw, r, err)
                return
            }
            if u.Scheme != "http" && u.Scheme != "https" {
                rw.WriteHeader(http.StatusBadRequest)
                fmt.Fprintf(rw, "URL must contain scheme, e.g. `http://` or `https://`.")
                return
            }
            var (
                link Link
                h    = strings.Trim(r.URL.Path, "/")
            )
            if h != "" {
                link, err = c.db.NewLinkWithShortLink(u, h)

            } else {
                link, err = c.db.NewLink(u)
            }
            if err != nil {
                c.Err(rw, r, err)
                return
            }
            rw.Header().Set("X-Delete-With", link.Del)
            rw.WriteHeader(http.StatusFound)
            fmt.Fprintf(rw, "%s/%s", c.url, link.Smol)
            return
        } else {
            switch st {

            case "":
                data := map[string]interface{}{
                    "URL":  c.url,
                    "Demo": c.demo,
                    "Copy": c.copy,
                }
                if err := c.tmpl.Execute(rw, data); err != nil {
                    c.Err(rw, r, err)
                    return
                }
                return

            case "/favicon.ico":
                http.NotFound(rw, r)
                return

            default:
                link, err := c.db.GetLink(strings.TrimLeft(r.URL.Path, "/"))
                if err != nil {
                    c.Err(rw, r, err)
                    return
                }
                http.Redirect(rw, r, link.Big, http.StatusPermanentRedirect)
                return

            }
        }

	case http.MethodPost:
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			c.Err(rw, r, err)
			return
		}
		u, err := url.Parse(string(b))
		if err != nil {
			c.Err(rw, r, err)
			return
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "URL must contain scheme, e.g. `http://` or `https://`.")
			return
		}
		var (
			link Link
			h    = strings.Trim(r.URL.Path, "/")
		)
		if h != "" {
			link, err = c.db.NewLinkWithShortLink(u, h)

		} else {
			link, err = c.db.NewLink(u)
		}
		if err != nil {
			c.Err(rw, r, err)
			return
		}
		rw.Header().Set("X-Delete-With", link.Del)
		rw.WriteHeader(http.StatusFound)
		fmt.Fprintf(rw, "%s/%s", c.url, link.Smol)
		return

	case http.MethodDelete:
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			c.Err(rw, r, err)
			return
		}
		if len(b) < 1 {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "Must include deletion key in DELETE body.")
			return
		}
		var (
			smol = strings.TrimSpace(strings.TrimLeft(r.URL.Path, "/"))
			del  = strings.TrimSpace(string(b))
		)
		if err := c.db.DelLink(smol, del); err != nil {
			c.Err(rw, r, err)
			return
		}
		rw.WriteHeader(http.StatusNoContent)
		return

	}

	http.NotFound(rw, r)
}

func main() {
	var (
		logPrefix         = "link: "
		startupLogger     = log.New(os.Stdout, logPrefix, 0)
		applicationLogger = log.New(ioutil.Discard, logPrefix, 0)
		v                 = flag.Bool("v", false, "verbose logging")
		demo              = flag.Bool("demo", false, "turn on demo mode")
		port              = flag.Uint("port", 8080, "port to listen on")
		dbFilePath        = flag.String("db", "", "sqlite database filepath: required")
        url               = flag.String("url", "", "URL which the server will be running on: required")
		hashSeed          = flag.String("seed", "", "hash seed: required")
		copy              = flag.String("copy", "", "copyright information")
	)
	flag.Parse()
	if *dbFilePath == "" || *url == "" || *hashSeed == "" {
		flag.Usage()
		return
	}
	if *v {
		applicationLogger = log.New(os.Stdout, logPrefix, 0)
	}
	retry, err := NewRetry(3)
	if err != nil {
		startupLogger.Fatal(err)
		return
	}
	db, err := NewDB(applicationLogger, *dbFilePath, *hashSeed, retry)
	if err != nil {
		startupLogger.Fatal(err)
		return
	}
	tmpl, err := template.New("").Parse(indexTemplate)
	if err != nil {
		startupLogger.Fatal(err)
		return
	}
	http.Handle("/", NewController(applicationLogger, db, *demo, *url, *copy, tmpl))
	startupLogger.Println("listening on port", *port)
	startupLogger.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}

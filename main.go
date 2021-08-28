// A URL Shortener called Link.
// Copyright (C) 2021 swurl@swurl.xyz
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
	"bytes"
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
)

//go:embed index.html
var indexTemplate string

type NotFoundError struct {
	Err string
}

func (e *NotFoundError) Error() string {
	return e.Err
}

type Link struct {
	Big  []byte
	Smol string
	Del  []byte
}

func GetHashShortLink(s fmt.Stringer) (string, error) {
	var (
		h      = maphash.Hash{}
		_, err = h.WriteString(s.String())
	)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.TrimLeft(fmt.Sprintf("%#x\n", h.Sum64()), "0x")), nil
}

type controller struct {
	log       *log.Logger
	linksPath string
	delPath   string
	demo      bool
	url, copy string
	hashSeed  string
	tmpl      *template.Template
}

func NewController(logger *log.Logger, path string, demo bool, url, copy string, hashSeed string, tmpl *template.Template) controller {
	return controller{logger, path + "links/", path + "del/", demo, strings.TrimRight(url, "/"), copy, hashSeed, tmpl}
}

func (c controller) Exists(name string) bool {
	if _, err := os.Stat(c.linksPath + name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func (c controller) WriteLink(link Link) error {
	if !c.Exists(link.Smol) {
		err := ioutil.WriteFile(c.linksPath+link.Smol, link.Big, 0644)
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(c.delPath+link.Smol, link.Del, 0644)
		if err != nil {
			return err
		}
		return nil
	}
	return errors.New("This short link already exists.")
}

func (c controller) GetHashDeleteKey(s fmt.Stringer) []byte {
	return []byte(strings.TrimSpace(fmt.Sprintf("%x", md5.Sum([]byte(c.hashSeed+s.String()+strconv.FormatInt(time.Now().Unix(), 10))))))
}

func (c controller) GetLink(smol string) (Link, error) {
	if c.Exists(smol) {
		big, err := ioutil.ReadFile(c.linksPath + smol)
		if err != nil {
			return Link{}, err
		}
		del, err := ioutil.ReadFile(c.delPath + smol)
		if err != nil {
			return Link{}, err
		}
		return Link{Big: big, Smol: smol, Del: del}, nil
	}
	return Link{}, &NotFoundError{"This short link does not exist."}
}

func (c controller) DelLink(smol, del string) error {
	link, err := c.GetLink(smol)
	if err != nil {
		return err
	}
	if bytes.Compare(link.Del, []byte(del)) != 0 {
		return errors.New("Incorrect deletion key.")
	}
	err = os.Remove(c.linksPath + smol)
	if err != nil {
		return err
	}
	err = os.Remove(c.delPath + smol)
	return err
}

func (c controller) NewShortLink(u *url.URL, hash string) (link Link, err error) {
	link = Link{Big: []byte(u.String()), Smol: hash, Del: c.GetHashDeleteKey(u)}
	err = c.WriteLink(link)
	if err != nil {
		return link, err
	}
	return link, err
}

func (c controller) NewLink(u *url.URL) (Link, error) {
	h, err := GetHashShortLink(u)
	if err != nil {
		return Link{}, err
	}
	return c.NewShortLink(u, h)
}

func (c controller) Err(rw http.ResponseWriter, r *http.Request, err error) {
	var nferr *NotFoundError
	if errors.As(err, &nferr) {
		rw.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(rw, "%s", err)
		return
	}
	c.log.Println(err)
	rw.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(rw, "%s", err)
}

func (c controller) CreateShortLink(rw http.ResponseWriter, r *http.Request, rq string) {
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
		link, err = c.NewShortLink(u, h)
	} else {
		link, err = c.NewLink(u)
	}
	if err != nil {
		c.Err(rw, r, err)
		return
	}
	rw.Header().Set("X-Delete-With", string(link.Del))
	rw.WriteHeader(http.StatusFound)
	fmt.Fprintf(rw, "%s/%s", c.url, link.Smol)
	return
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
			c.CreateShortLink(rw, r, rq)
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
				link, err := c.GetLink(strings.TrimLeft(r.URL.Path, "/"))
				if err != nil {
					c.Err(rw, r, err)
					return
				}
				http.Redirect(rw, r, string(link.Big), http.StatusPermanentRedirect)
				return
			}
		}

	case http.MethodPost:
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			c.Err(rw, r, err)
			return
		}
		c.CreateShortLink(rw, r, string(b))
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
		if err := c.DelLink(smol, del); err != nil {
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
		dirPath           = flag.String("path", "", "path to directory used for storing data: required")
		url               = flag.String("url", "", "URL which the server will be running on: required")
		hashSeed          = flag.String("seed", "", "hash seed: required")
		copy              = flag.String("copy", "", "copyright information")
	)
	flag.Parse()
	if *dirPath == "" || *url == "" || *hashSeed == "" {
		flag.Usage()
		return
	}
	if *v {
		applicationLogger = log.New(os.Stdout, logPrefix, 0)
	}
	tmpl, err := template.New("").Parse(indexTemplate)
	if err != nil {
		startupLogger.Fatal(err)
		return
	}
	os.MkdirAll(*dirPath+"links", 0644)
	os.MkdirAll(*dirPath+"del", 0644)
	http.Handle("/", NewController(applicationLogger, *dirPath, *demo, *url, *copy, *hashSeed, tmpl))
	startupLogger.Println("listening on port", *port)
	startupLogger.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}

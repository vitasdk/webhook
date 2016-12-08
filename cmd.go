package main

// AGPLv2+
// Copyright (c) 2016 Josh de kock
// jk, just MIT (expat)

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"github.com/bmatsuo/go-jsontree"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// include https://github.com/phayes/hookserve/blob/master/LICENSE.md

var ErrInvalidEventFormat = errors.New("Unable to parse event string. Invalid Format.")
var SupportedEvents = []string{"pull_request", "push", "ping"}

// repos to trigger a group
var repos = map[string]string{
	"vitasdk/buildscripts":     "vitasdk",
	"vitasdk/newlib":           "vitasdk",
	"vitasdk/vita-headers":     "vitasdk",
	"vitasdk/vita-toolchain":   "vitasdk",
	"vitasdk/vita-samples":     "vitasdk",
	"vitasdk/pthread-embedded": "vitasdk",
}

// groups end trigger point
var groups = map[string]string{
	"vitasdk": "vitasdk/test",
}

type Server struct {
	Port       int
	Path       string
	Secret     string
	IgnoreTags bool
}

func NewServer() *Server {
	return &Server{
		Port:       80,
		Path:       "/",
		IgnoreTags: true,
	}
}

func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(":"+strconv.Itoa(s.Port), s)
}

func (s *Server) GoListenAndServe() {
	go func() {
		err := s.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()
}

func (s *Server) ignoreRef(rawRef string) bool {
	if rawRef[:10] == "refs/tags/" && !s.IgnoreTags {
		return false
	}
	return rawRef[:11] != "refs/heads/"
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()
	defer func() {
		// recover from panic if one occured. Set err to nil otherwise.
		if recover() != nil {
			http.Error(w, "recovered from panic", 500)
		}
	}()
	if req.Method != "POST" {
		http.Error(w, "405 Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if req.URL.Path != s.Path {
		http.Error(w, "404 Not found", http.StatusNotFound)
		return
	}

	eventType := req.Header.Get("X-GitHub-Event")
	if eventType == "" {
		http.Error(w, "400 Bad Request - Missing X-GitHub-Event Header", http.StatusBadRequest)
		return
	}

	for _, event := range SupportedEvents {
		if event == eventType {
			goto okevent
		}
	}
	http.Error(w, "400 Bad Request - Unknown Event Type "+eventType, http.StatusBadRequest)
	return
okevent:

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If we have a Secret set, we should check the MAC
	if s.Secret != "" {
		sig := req.Header.Get("X-Hub-Signature")

		if sig == "" {
			http.Error(w, "403 Forbidden - Missing X-Hub-Signature required for HMAC verification", http.StatusForbidden)
			return
		}

		mac := hmac.New(sha1.New, []byte(s.Secret))
		mac.Write(body)
		expectedMAC := mac.Sum(nil)
		expectedSig := "sha1=" + hex.EncodeToString(expectedMAC)
		if !hmac.Equal([]byte(expectedSig), []byte(sig)) {
			http.Error(w, "403 Forbidden - HMAC verification failed", http.StatusForbidden)
			return
		}
	}

	request := jsontree.New()
	err = request.UnmarshalJSON(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch eventType {
	case "push":
		triggers := make(map[string]bool)
		reponame, err := request.Get("repository").Get("full_name").String()
		log.Printf("Got push trigger from %s", reponame)
		for name, group := range repos {
			if name == reponame {
				if endpoint, ok := groups[group]; ok {
					triggers[endpoint] = true
				}
			}
		}
		if err != nil {
			http.Error(w, "rip couldnt get repo name", 500)
		}
		for trigger, _ := range triggers {
			go func() {
				log.Printf("executing %s", trigger)
				if _, err = exec.Command("./trigger", trigger).Output(); err != nil {
					http.Error(w, "rip couldnt run trigger command", 500)
				} else {
					w.Write([]byte("k."))
				}
			}()
		}
	case "ping":
		w.Write([]byte("pong"))
	default:
		http.Error(w, "Not implemented.", 501)
	}
}

func main() {
	server := NewServer()
	server.Port = 5019
	server.Secret = os.Getenv("GH_SECRET")
	if len(strings.TrimSpace(server.Secret)) < 1 {
		panic("GH_SECRET unset")
	}
	server.ListenAndServe()
}

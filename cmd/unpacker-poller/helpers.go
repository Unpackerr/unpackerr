package main

import (
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"

	"github.com/davidnewhall/unpacker-poller/deluge"
	"github.com/davidnewhall/unpacker-poller/starr"
)

// Returns all the files in a path.
func getFileList(path string) (files []string) {
	if fileList, err := ioutil.ReadDir(path); err == nil {
		for _, file := range fileList {
			if !file.IsDir() {
				files = append(files, filepath.Join(path, file.Name()))
			}
		}
	} else {
		log.Println("Error reading path", path, err.Error())
	}
	return
}

// Returns all the strings that are in slice2 but not in slice1.
// Finds new files in a file list from a path. ie. those we extracted.
func difference(slice1 []string, slice2 []string) (diff []string) {
	for _, s2 := range slice2 {
		found := false
		for _, s1 := range slice1 {
			if s1 == s2 {
				found = true
				break
			}
		}
		if !found {
			// String not found.
			diff = append(diff, s2)
		}
	}
	return diff
}

// Returns the (a) .rar file from a path.
func findRarFile(path string) string {
	for _, file := range getFileList(path) {
		if strings.HasSuffix(file, ".rar") {
			return file
		}
	}
	return ""
}

/*
  The following functions pull data from the internal map and slices.
  Those slices and map are populated by their respective go routines.
*/

// gets a radarr queue item based on name. returns first match
// there may be more than one match if it involes an "episode pack" (full season)
func (r *runningData) getSonarQitem(name string) (s starr.SonarQueue) {
	r.sonS.RLock()
	defer r.sonS.RUnlock()
	for i := range r.SonarrQ {
		if r.SonarrQ[i].Title == name {
			return *r.SonarrQ[i]
		}
	}
	return s
}

// gets a radarr queue item based on name. returns first match
func (r *runningData) getRadarQitem(name string) (s starr.RadarQueue) {
	r.radS.RLock()
	defer r.radS.RUnlock()
	for i := range r.RadarrQ {
		if r.RadarrQ[i].Title == name {
			return *r.RadarrQ[i]
		}
	}
	return s
}

// Get a Deluge transfer based on name.
func (r *runningData) getXfer(name string) (d deluge.XferStatus) {
	r.delS.RLock()
	defer r.delS.RUnlock()
	for _, data := range r.Deluge {
		if data.Name == name {
			return *data
		}
	}
	return d
}

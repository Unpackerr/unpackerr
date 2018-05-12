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
			files = append(files, filepath.Join(path, file.Name()))
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

// Returns all the rar files in a path.
func findRarFiles(path string) (files []string) {
	if fileList, err := ioutil.ReadDir(path); err == nil {
		for _, file := range fileList {
			if file.IsDir() {
				// Recurse.
				files = append(files, findRarFiles(filepath.Join(path, file.Name()))...)
			} else if strings.HasSuffix(file.Name(), ".rar") {
				files = append(files, filepath.Join(path, file.Name()))
			}
		}
	}
	return
}

/*
  The following functions pull data from the internal map and slices.
*/

// gets a radarr queue item based on name. returns first match
// there may be more than one match if it involes an "episode pack" (full season)
func (r *RunningData) getSonarQitem(name string) (s starr.SonarQueue) {
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
func (r *RunningData) getRadarQitem(name string) (s starr.RadarQueue) {
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
func (r *RunningData) getXfer(name string) (d deluge.XferStatus) {
	r.delS.RLock()
	defer r.delS.RUnlock()
	for _, data := range r.Deluge {
		if data.Name == name {
			return *data
		}
	}
	return d
}

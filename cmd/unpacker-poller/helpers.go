package main

import (
	"io/ioutil"
	"log"
	"os"
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
			} else if name := strings.ToLower(file.Name()); strings.HasSuffix(name, ".rar") {
				// Some archives are named poorly. Only return part01 or part001, not all.
				m, _ := filepath.Match("*.part[0-9]*.rar", name)
				if !m || strings.HasSuffix(name, ".part01.rar") || strings.HasSuffix(name, ".part001.rar") || strings.HasSuffix(name, ".part1.rar") {
					files = append(files, filepath.Join(path, file.Name()))
				}
			}
		}
	}
	return
}

// Moves files then removes the folder they were in.
// Returns the new file paths.
func moveFiles(fromPath string, toPath string) ([]string, error) {
	files := getFileList(fromPath)
	var keepErr error
	for i, file := range files {
		newFile := filepath.Join(toPath, filepath.Base(file))
		if err := os.Rename(file, newFile); err != nil {
			keepErr = err
			log.Printf("Error Renaming: %v to %v: %v", file, newFile, err.Error())
			// keep trying.
			continue
		}
		DeLogf("Renamed File: %v -> %v", file, newFile)
		files[i] = newFile
	}
	if errr := os.Remove(fromPath); errr != nil {
		log.Printf("Error Removing Folder: %v: %v", fromPath, errr.Error())
		// If we made it this far, it's ok.
	} else {
		DeLogf("Removed Folder: %v", fromPath)
	}
	// Since this is the last step, we tried to rename all the files, bubble the
	// os.Rename error up, so it gets flagged as failed. It may have worked, but
	// it should get attention.
	return files, keepErr
}

// Deletes extracted files after Sonarr/Radarr imports them.
func deleteFiles(name string, files []string) error {
	var keepErr error
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			keepErr = err
			log.Printf("Error Deleting %v: %v", file, err.Error())
			continue
		}
		log.Println("Deleted:", file)
	}
	return keepErr
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

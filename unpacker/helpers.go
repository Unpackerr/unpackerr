package unpacker

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golift.io/starr"
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
		var found bool

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

// FindRarFiles returns all the rar files in a path. This attempts to grab only the first
// file in a multi-part archive. Sometimes there are multiple archives, so if the archive
// does not have "part" followed by a number in the name, then it will be considered
// an independent archive. Some packagers seem to use different naming schemes, so this
// will need to be updated as time progresses. So far it's working well. -dn2@8/3/19
func FindRarFiles(path string) []string {
	fileList, err := ioutil.ReadDir(path)
	if err != nil {
		return nil
	}

	var hasrar bool

	var files []string

	// Check (save) if the current path has any rar files.
	if r, err := filepath.Glob(filepath.Join(path, "*.rar")); err == nil && len(r) > 0 {
		hasrar = true
	}

	for _, file := range fileList {
		switch lowerName := strings.ToLower(file.Name()); {
		case file.IsDir(): // Recurse.
			files = append(files, FindRarFiles(filepath.Join(path, file.Name()))...)
		case strings.HasSuffix(lowerName, ".rar"):
			// Some archives are named poorly. Only return part01 or part001, not all.
			m, _ := filepath.Match("*.part[0-9]*.rar", lowerName)
			// This if statements says:
			// If the current file does not have "part0-9" in the name, add it to our list (all .rar files).
			// If it does have "part0-9" in the name, then make sure it's part 1.
			if !m || strings.HasSuffix(lowerName, ".part01.rar") ||
				strings.HasSuffix(lowerName, ".part001.rar") ||
				strings.HasSuffix(lowerName, ".part1.rar") {
				files = append(files, filepath.Join(path, file.Name()))
			}
		case !hasrar && strings.HasSuffix(lowerName, ".r00"):
			// Accept .r00 as the first file file if no .rar files are present in the path.
			files = append(files, filepath.Join(path, file.Name()))
		}
	}

	return files
}

// Moves files then removes the folder they were in.
// Returns the new file paths.
func (u *Unpackerr) moveFiles(fromPath string, toPath string) ([]string, error) {
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

		u.DeLogf("Renamed File: %v -> %v", file, newFile)

		files[i] = newFile
	}

	if err := os.Remove(fromPath); err != nil {
		log.Printf("Error Removing Folder: %v: %v", fromPath, err)
	} else {
		// If we made it this far, it's ok.
		u.DeLogf("Removed Folder: %v", fromPath)
	}

	// Since this is the last step, we tried to rename all the files, bubble the
	// os.Rename error up, so it gets flagged as failed. It may have worked, but
	// it should get attention.
	return files, keepErr
}

// Deletes extracted files after Sonarr/Radarr imports them.
func deleteFiles(files []string) {
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			log.Printf("Error Deleting %v: %v", file, err.Error())
			continue
		}

		log.Println("Deleted:", file)
	}
}

/*
  The following functions pull data from the internal map and slices.
*/

func (u *Unpackerr) getSonarQitem(name string) *starr.SonarQueue {
	getItem := func(name string, sonarr *sonarrConfig) *starr.SonarQueue {
		sonarr.RLock()
		defer sonarr.RUnlock()

		for i := range sonarr.List {
			if sonarr.List[i].Title == name {
				return sonarr.List[i]
			}
		}

		return nil
	}

	for _, sonarr := range u.Sonarr {
		if s := getItem(name, sonarr); s != nil {
			return s
		}
	}

	return nil
}

// gets a radarr queue item based on name. returns first match
func (u *Unpackerr) getRadarQitem(name string) *starr.RadarQueue {
	getItem := func(name string, radarr *radarrConfig) *starr.RadarQueue {
		radarr.RLock()
		defer radarr.RUnlock()

		for i := range radarr.List {
			if radarr.List[i].Title == name {
				return radarr.List[i]
			}
		}

		return nil
	}

	for _, radarr := range u.Radarr {
		if s := getItem(name, radarr); s != nil {
			return s
		}
	}

	return nil
}

// gets a lidarr queue item based on name. returns first match
func (u *Unpackerr) getLidarQitem(name string) *starr.LidarrRecord {
	getItem := func(name string, lidarr *lidarrConfig) *starr.LidarrRecord {
		lidarr.RLock()
		defer lidarr.RUnlock()

		for i := range lidarr.List {
			if lidarr.List[i].Title == name {
				return lidarr.List[i]
			}
		}

		return nil
	}

	for _, lidarr := range u.Lidarr {
		if s := getItem(name, lidarr); s != nil {
			return s
		}
	}

	return nil
}

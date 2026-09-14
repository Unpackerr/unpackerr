package unpackerr

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	homedir "github.com/mitchellh/go-homedir"
)

var (
	errLogNotFound = errors.New("log file not found")
	errLogLive     = errors.New("live logs cannot be downloaded")
)

// LogFileInfos holds metadata about files in the log directories.
type LogFileInfos struct {
	Dirs []string       `json:"dirs"`
	Size int64          `json:"size"`
	List []*LogFileInfo `json:"list"`
}

// LogFileInfo is one current or rotated log file.
type LogFileInfo struct {
	ID   string    `json:"id"`
	Name string    `json:"name"`
	Path string    `json:"path"`
	Size int64     `json:"size"`
	Time time.Time `json:"time"`
	Mode string    `json:"mode"`
	Used bool      `json:"used"`
	User string    `json:"user"`
}

type logLinesBody struct {
	Text string `json:"text"`
}

func (u *Unpackerr) registerLogRoutes() {
	base := path.Join(u.Webserver.URLBase, "api")
	join := func(b string) string { return path.Join(base, b) }

	u.Webserver.handleGet(join("logs"), u.requirePerm(PermReadSystemLogs, u.logsListHandler))
	u.Webserver.handleGet(join("logs/{id}/download"), u.requirePerm(PermReadSystemLogs, u.logsDownloadHandler))
	u.Webserver.handleGet(join("logs/{id}"), u.requirePerm(PermReadSystemLogs, u.logsGetHandler))
}

func (u *Unpackerr) logsListHandler(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, u.logFileInfos())
}

func (u *Unpackerr) logsGetHandler(response http.ResponseWriter, request *http.Request) {
	info := u.findLogFile(request.PathValue("id"))
	if info == nil {
		writeJSON(response, http.StatusNotFound, map[string]string{"error": errLogNotFound.Error()})
		return
	}

	lines, _ := strconv.Atoi(request.URL.Query().Get("lines"))
	skip, _ := strconv.Atoi(request.URL.Query().Get("skip"))
	lines = clampLogLines(lines)

	if skip < 0 {
		skip = 0
	}

	text, err := u.readLogLines(info, lines, skip)
	if err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(response, http.StatusOK, logLinesBody{Text: text})
}

func (u *Unpackerr) logsDownloadHandler(response http.ResponseWriter, request *http.Request) {
	info := u.findLogFile(request.PathValue("id"))
	if info == nil {
		writeJSON(response, http.StatusNotFound, map[string]string{"error": errLogNotFound.Error()})
		return
	}

	if isSyntheticLog(info) || info.Path == "" {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": errLogLive.Error()})
		return
	}

	file, err := os.Open(info.Path)
	if err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer file.Close()

	response.Header().Set("Content-Type", "application/zip")
	response.Header().Set("Content-Disposition", `attachment; filename="`+info.Name+`.zip"`)

	zipWriter := zip.NewWriter(response)
	defer zipWriter.Close()

	entry, err := zipWriter.Create(info.Name)
	if err != nil {
		return
	}

	_, _ = io.Copy(entry, file)
}

func (u *Unpackerr) logFileInfos() *LogFileInfos {
	paths := u.activeLogPaths()
	infos := GetFilePaths(paths...)

	// App stdout (id live) is independent of HTTP/file logs; keep it even when
	// those files exist, and as a fallback when a configured file is missing.
	if u.LogFile == "" || len(infos.List) == 0 {
		infos.List = append([]*LogFileInfo{liveLogInfo()}, infos.List...)
	}

	infos.List = append([]*LogFileInfo{u.recentErrorsInfo()}, infos.List...)

	return infos
}

func liveLogInfo() *LogFileInfo {
	return &LogFileInfo{
		ID:   liveLogID,
		Name: "live",
		Path: "(stdout)",
		Used: true,
		Time: time.Now().Round(time.Second),
	}
}

func (u *Unpackerr) recentErrorsInfo() *LogFileInfo {
	info := &LogFileInfo{
		ID:   errorLogID,
		Name: "recent errors",
		Path: "(errors)",
		Used: true,
		Time: time.Now().Round(time.Second),
	}

	if rows := u.hub.errorSnapshot(); len(rows) > 0 {
		info.Time = rows[len(rows)-1].At.Round(time.Second)
	}

	return info
}

func isSyntheticLog(info *LogFileInfo) bool {
	if info == nil {
		return false
	}

	switch info.ID {
	case liveLogID, errorLogID:
		return true
	}

	switch info.Path {
	case "(stdout)", "(errors)":
		return true
	}

	return false
}

func (u *Unpackerr) activeLogPaths() []string {
	var paths []string

	if u.LogFile != "" {
		paths = append(paths, u.LogFile)
	}

	if u.Webserver != nil && u.Webserver.LogFile != "" {
		paths = append(paths, u.Webserver.LogFile)
	}

	return paths
}

func (u *Unpackerr) findLogFile(id string) *LogFileInfo {
	for _, item := range u.logFileInfos().List {
		if item != nil && item.ID == id {
			return item
		}
	}

	return nil
}

func (u *Unpackerr) readLogLines(info *LogFileInfo, count, skip int) (string, error) {
	if info.ID == errorLogID || info.Path == "(errors)" {
		return formatErrorLines(u.hub.errorSnapshot(), count, skip), nil
	}

	if info.ID == liveLogID || info.Path == "(stdout)" {
		var lines []string
		if u.appLogTee != nil {
			lines = lastN(u.appLogTee.ring.snapshot(), count+skip)
		}

		if skip >= len(lines) {
			return "", nil
		}

		if skip > 0 {
			lines = lines[:len(lines)-skip]
		}

		if len(lines) > count {
			lines = lines[len(lines)-count:]
		}

		return strings.Join(lines, "\n"), nil
	}

	raw, err := getLinesFromFile(info.Path, count, skip)
	if err != nil {
		return "", err
	}

	return string(raw), nil
}

func formatErrorLines(frames []errorFrame, count, skip int) string {
	if skip >= len(frames) {
		return ""
	}

	frames = frames[:len(frames)-skip]
	if count > 0 && len(frames) > count {
		frames = frames[len(frames)-count:]
	}

	lines := make([]string, len(frames))
	for i, frame := range frames {
		lines[i] = formatErrorLine(frame)
	}

	return strings.Join(lines, "\n")
}

func formatErrorLine(frame errorFrame) string {
	return frame.At.UTC().Format(time.RFC3339) + " " + frame.Msg
}

func clampLogLines(lines int) int {
	if lines <= 0 {
		return defaultLogLines
	}

	if lines > maxLogLines {
		return maxLogLines
	}

	return lines
}

func encodeLogFileID(logPath string) string {
	if logPath == "" {
		return liveLogID
	}

	if expanded, err := homedir.Expand(logPath); err == nil {
		logPath = expanded
	}

	if abs, err := filepath.Abs(logPath); err == nil {
		logPath = abs
	}

	return base64.RawURLEncoding.EncodeToString([]byte(logPath))
}

// GetFilePaths returns files in the same folder with the same extension as the given paths.
func GetFilePaths(files ...string) *LogFileInfos {
	similar, output := getSimilarFiles(files)
	used := make(map[string]os.FileInfo)

	for _, name := range files {
		if name == "" {
			continue
		}

		if expanded, err := homedir.Expand(name); err == nil {
			name = expanded
		}

		if stat, err := os.Stat(name); err == nil {
			used[name] = stat
		}
	}

	idx := 0

	for filePath, fileInfo := range similar {
		inUse := false

		for _, file := range used {
			if inUse = os.SameFile(fileInfo, file); inUse {
				break
			}
		}

		if abs, err := filepath.Abs(filePath); err == nil {
			filePath = abs
		}

		output.List[idx] = &LogFileInfo{
			ID:   encodeLogFileID(filePath),
			Name: fileInfo.Name(),
			Path: filePath,
			Size: fileInfo.Size(),
			Time: fileInfo.ModTime().Round(time.Second),
			Mode: fmt.Sprintf("%s (%o)", fileInfo.Mode(), fileInfo.Mode()),
			Used: inUse,
			User: getFileOwner(fileInfo),
		}
		idx++
		output.Size += fileInfo.Size()
	}

	sort.Sort(output)

	return output
}

func (l *LogFileInfos) Len() int { return len(l.List) }

func (l *LogFileInfos) Swap(i, j int) { l.List[i], l.List[j] = l.List[j], l.List[i] }

func (l *LogFileInfos) Less(i, j int) bool {
	return l.List[i].Time.After(l.List[j].Time)
}

func getSimilarFiles(files []string) (map[string]os.FileInfo, *LogFileInfos) {
	similar := make(map[string]os.FileInfo)
	ignored := make(map[string]struct{})
	dirs := make(map[string]struct{})

	for _, filePath := range files {
		if filePath == "" {
			continue
		}

		ext := filepath.Ext(filePath)
		if ext == "" {
			continue
		}

		if expanded, err := homedir.Expand(filePath); err == nil {
			filePath = expanded
		}

		matches, err := filepath.Glob(filepath.Join(filepath.Dir(filePath), "*"+ext))
		if err != nil {
			continue
		}

		for _, file := range matches {
			dirs[filepath.Dir(file)] = struct{}{}
			if _, skip := ignored[file]; skip || similar[file] != nil {
				continue
			}

			info, err := os.Stat(file)
			if err != nil || info.IsDir() {
				ignored[file] = struct{}{}
				continue
			}

			similar[file] = info
		}
	}

	list := make([]string, 0, len(dirs))
	for dir := range dirs {
		list = append(list, dir)
	}

	sort.Strings(list)

	return similar, &LogFileInfos{List: make([]*LogFileInfo, len(similar)), Dirs: list}
}

func getLinesFromFile(filePath string, count, skip int) ([]byte, error) {
	fileHandle, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer fileHandle.Close()

	stat, err := fileHandle.Stat()
	if err != nil {
		return nil, fmt.Errorf("stating open file: %w", err)
	}

	if stat.Size() == 0 || count <= 0 {
		return nil, nil
	}

	return readFileTail(fileHandle, stat.Size(), count, skip)
}

func readFileTail(fileHandle *os.File, fileSize int64, count, skip int) ([]byte, error) {
	var (
		output   bytes.Buffer
		location int64
		found    int
		char     = make([]byte, 1)
	)

	output.Grow(count * 150) //nolint:mnd

	for {
		location--
		if _, err := fileHandle.Seek(location, io.SeekEnd); err != nil {
			return nil, fmt.Errorf("seeking open file: %w", err)
		}

		if _, err := fileHandle.Read(char); err != nil {
			return nil, fmt.Errorf("reading open file: %w", err)
		}

		if location != -1 && char[0] == '\n' {
			found++
		}

		if skip == 0 || found >= skip {
			output.WriteByte(char[0])
		}

		if found >= count+skip || location == -fileSize {
			out := revBytes(output)
			if len(out) > 0 && out[0] == '\n' {
				return out[1:], nil
			}

			return out, nil
		}
	}
}

func revBytes(output bytes.Buffer) []byte {
	data := output.Bytes()
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}

	return data
}

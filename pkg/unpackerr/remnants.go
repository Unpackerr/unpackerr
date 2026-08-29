package unpackerr

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golift.io/xtractr"
)

const (
	remnantSuffix    = ".remnant"
	maxRemnantCopies = 999
)

// Errors produced by this file.
var (
	// ErrInvalidRemnantAction is returned when remnant_action is not rename, delete, or off.
	ErrInvalidRemnantAction = errors.New("invalid remnant_action")
	errNoRemnantName        = errors.New("no unused remnant name")
	errNotDirectory         = errors.New("not a directory")
)

// remnantAction normalizes remnant_action. Empty and unknown values become
// "rename"; validateRemnantAction rejects the unknowns at startup.
func remnantAction(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "delete":
		return "delete"
	case "off":
		return "off"
	default:
		return "rename"
	}
}

func (u *Unpackerr) validateRemnantAction() error {
	s := strings.TrimSpace(u.RemnantAction)
	if s != "" && remnantAction(s) != strings.ToLower(s) {
		return fmt.Errorf("%w: %q (want rename, delete, or off)", ErrInvalidRemnantAction, u.RemnantAction)
	}

	u.RemnantAction = remnantAction(s)

	return nil
}

// archiveFileDest is the directory xtractr moveFiles uses when Path is an
// archive file: it strips the extension so the extract lands in a sibling
// folder. Children of that folder are not in the parent listing.
func archiveFileDest(path string) string {
	if !xtractr.IsArchiveFile(path) {
		return ""
	}

	dest := strings.TrimSuffix(path, filepath.Ext(path))
	if dest == "" || dest == path {
		return ""
	}

	return dest
}

// archiveSnapshotPaths is the search path plus every folder FindCompressedFiles
// already keyed (those are xtractr's per-archive dests / FinalDests keys).
// Archive-file keys also include the extension-stripped sibling dest.
func archiveSnapshotPaths(root string, archives xtractr.ArchiveList) []string {
	paths := make([]string, 0, len(archives)*2+1)
	if root != "" {
		paths = append(paths, root)
	}

	for dir := range archives {
		if dir != "" {
			paths = append(paths, dir)
		}

		if dest := archiveFileDest(dir); dest != "" {
			paths = append(paths, dest)
		}
	}

	return paths
}

// snapshotDir returns the directory to list for path. An archive file uses its parent.
func snapshotDir(path string) string {
	path = filepath.Clean(path)
	if path == "" || path == "." {
		return ""
	}

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return filepath.Dir(path)
	}

	return path
}

// keepDirSnapshot returns the first pre-extraction listing. Keys are cleaned
// full paths of the immediate children of each dest folder (same listing
// xtractr.GetFileList uses). Retries must not recapture leftovers that failed
// to clear. A ReadDir failure returns (nil, err) so callers leave PreFiles
// unset and handleRemnants will not classify that item.
func keepDirSnapshot(existing map[string]os.FileInfo, paths ...string) (map[string]os.FileInfo, error) {
	if existing != nil {
		return existing, nil
	}

	out := make(map[string]os.FileInfo)
	seen := make(map[string]struct{}, len(paths))

	for _, path := range paths {
		root := snapshotDir(path)
		if root == "" || root == "." {
			continue
		}

		if _, dup := seen[root]; dup {
			continue
		}

		seen[root] = struct{}{}

		if err := keepDirChildren(out, root); err != nil {
			return nil, err
		}
	}

	return out, nil
}

func keepDirChildren(out map[string]os.FileInfo, root string) error {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // dest does not exist yet; nothing there to protect.
		}

		return fmt.Errorf("os.Stat: %w", err)
	}

	if !info.IsDir() {
		// Windows ReadDir of a file is an empty listing, not an error.
		return fmt.Errorf("%w: %s", errNotDirectory, root)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("os.ReadDir: %w", err)
	}

	for _, entry := range entries {
		full := filepath.Clean(filepath.Join(root, entry.Name()))

		info, err := os.Lstat(full)
		if err != nil {
			out[full] = nil

			continue
		}

		out[full] = info
	}

	return nil
}

// isDownloadContent reports whether dest was present before extraction, by
// cleaned full path or by same-file identity (case-only differences on
// case-insensitive filesystems).
func isDownloadContent(preFiles map[string]os.FileInfo, dest string) bool {
	if _, ok := preFiles[filepath.Clean(dest)]; ok {
		return true
	}

	destInfo, err := os.Lstat(dest)
	if err != nil {
		return false
	}

	for _, info := range preFiles {
		if info != nil && os.SameFile(info, destInfo) {
			return true
		}
	}

	return false
}

func underFinalDests(path string, dests map[string]string) bool {
	if path == "" || len(dests) == 0 {
		return false
	}

	path = filepath.Clean(path)

	for _, dest := range dests {
		if dest == "" {
			continue
		}

		dest = filepath.Clean(dest)
		if path == dest || strings.HasPrefix(path, dest+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

// handleRemnants classifies Response.Refused against the pre-extraction
// snapshot of each FinalDests value. snapshot must come from keepDirSnapshot
// at queue time. It clears non-snapshot blockers per remnant_action and also
// rolls back sibling NewFiles when a restart will re-extract into them.
//
// ok is false when remnants do not change the normal success/error path
// (no refusals, no FinalDests, or no snapshot). When ok is true, WAITING
// means restart and EXTRACTFAILED means a remnant still blocks. Callers
// must set NoRetry when remnant_action is off so EXTRACTFAILED is not retried.
func (u *Unpackerr) handleRemnants(
	resp *xtractr.Response,
	snapshot map[string]os.FileInfo,
	retries uint,
) (ExtractStatus, bool) {
	if resp == nil || snapshot == nil || len(resp.Refused) == 0 || len(resp.FinalDests) == 0 {
		return 0, false
	}

	remnants := u.classifyRemnants(resp, snapshot)
	if len(remnants) == 0 {
		return 0, false
	}

	if remnantAction(u.RemnantAction) == "off" {
		for _, dest := range remnants {
			u.Printf("Interrupted-extraction remnant left in place (remnant_action=off): %s", dest)
		}

		return EXTRACTFAILED, true
	}

	if u.MaxRetries != 0 && retries >= u.MaxRetries {
		return EXTRACTFAILED, true
	}

	if u.clearRemnants(remnants) != len(remnants) {
		return EXTRACTFAILED, true
	}

	// Every blocker is gone and we are about to re-extract. Refused files were
	// never moved (xtractr discarded them), but sibling outputs from a partial
	// move are still in place and would refuse on the next attempt — remove the
	// ones that were not part of the download so the restart extracts cleanly.
	for _, newFile := range resp.NewFiles {
		if !underFinalDests(newFile, resp.FinalDests) || isDownloadContent(snapshot, newFile) {
			continue
		}

		if err := os.RemoveAll(newFile); err != nil {
			u.Errorf("Removing partial-move output %s before re-extract: %v", newFile, err)
		}
	}

	return WAITING, true
}

func (u *Unpackerr) classifyRemnants(resp *xtractr.Response, snapshot map[string]os.FileInfo) []string {
	var remnants []string

	seen := make(map[string]struct{}, len(resp.Refused))

	for _, refused := range resp.Refused {
		if _, dup := seen[refused.Dest]; dup {
			continue
		}

		seen[refused.Dest] = struct{}{}

		if !underFinalDests(refused.Dest, resp.FinalDests) {
			u.Debugf("Ignoring refused path outside extract dest: %s", refused.Dest)
			continue
		}

		if isDownloadContent(snapshot, refused.Dest) {
			u.Debugf("Keeping download file that blocked extraction: %s", refused.Dest)
			continue
		}

		remnants = append(remnants, refused.Dest)
	}

	return remnants
}

// clearRemnants removes or renames each remnant and returns the count cleared.
func (u *Unpackerr) clearRemnants(remnants []string) int {
	cleared := 0

	for _, dest := range remnants {
		if u.clearRemnant(dest) {
			cleared++
		}
	}

	return cleared
}

// clearRemnant removes or renames a file that blocked extraction and did not
// arrive with the download. Returns false if the blocker remains.
func (u *Unpackerr) clearRemnant(dest string) bool {
	info, err := os.Lstat(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return true // already gone; the retry extracts into the empty dest.
		}

		u.Errorf("Checking interrupted-extraction remnant %s: %v", dest, err)

		return false
	}

	if remnantAction(u.RemnantAction) == "delete" {
		remove := os.Remove
		if info.IsDir() {
			remove = os.RemoveAll
		}

		if err := remove(dest); err != nil {
			u.Errorf("Removing interrupted-extraction remnant %s: %v", dest, err)
			return false
		}

		u.Printf("Removed interrupted-extraction remnant: %s", dest)

		return true
	}

	target, err := unusedRemnantPath(dest)
	if err != nil {
		u.Errorf("Renaming interrupted-extraction remnant %s: %v", dest, err)
		return false
	}

	if err := os.Rename(dest, target); err != nil {
		u.Errorf("Renaming interrupted-extraction remnant %s -> %s: %v", dest, target, err)
		return false
	}

	u.Printf("Renamed interrupted-extraction remnant: %s -> %s (you may delete it)", dest, target)

	return true
}

func unusedRemnantPath(path string) (string, error) {
	for n := range maxRemnantCopies {
		candidate := path + remnantSuffix
		if n > 0 {
			candidate = fmt.Sprintf("%s%s.%d", path, remnantSuffix, n)
		}

		_, err := os.Lstat(candidate)
		if os.IsNotExist(err) {
			return candidate, nil
		}

		if err != nil {
			return "", fmt.Errorf("os.Lstat: %w", err)
		}
	}

	return "", fmt.Errorf("%w: %s", errNoRemnantName, path)
}

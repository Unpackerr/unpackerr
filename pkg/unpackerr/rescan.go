package unpackerr

import (
	"path/filepath"
	"slices"
	"strings"

	"golift.io/xtractr"
)

// A Syncthing download can keep growing after its first extract. Each extract
// that finishes adds its archives to Extract.Unpacked. From then on the item
// is searched as a follow-up in every status: EXTRACTED while the Starr app
// still lists it, and WAITING for a retry. A follow-up leaves out those
// archives and the files they produced, so a late archive or a retry never
// unpacks anything twice.

// hasUnpacked reports an item that a finished extract recorded archives for.
func hasUnpacked(item *Extract) bool {
	return item != nil && item.Syncthing && len(item.Unpacked) > 0
}

// rescans reports a finished Syncthing download the Starr app still lists.
// More archives can arrive for it, so it is searched on every pass and is
// not swept as a stale item. Caller must hold History.mu.
func (u *Unpackerr) rescans(name string, item *Extract) bool {
	return item.Status == EXTRACTED && hasUnpacked(item) && u.haveQitem(name, item.App)
}

// unpackedSet is what earlier extracts already handled for one item.
type unpackedSet struct {
	root     string              // the download path; the walk up from a file stops here.
	archives map[string]struct{} // archives already unpacked, every volume.
	heads    map[string]struct{} // the same archives, keyed by headKey.
	output   map[string]struct{} // files and folders those extracts moved into place.
}

// newUnpackedSet returns nil for an item with no finished extract.
func newUnpackedSet(item *Extract) *unpackedSet {
	if !hasUnpacked(item) {
		return nil
	}

	set := &unpackedSet{
		root:     filepath.Clean(item.Path),
		archives: make(map[string]struct{}, len(item.Unpacked)),
		heads:    make(map[string]struct{}, len(item.Unpacked)),
		output:   make(map[string]struct{}),
	}

	for _, path := range item.Unpacked {
		set.archives[filepath.Clean(path)] = struct{}{}
		set.heads[headKey(path)] = struct{}{}
	}

	if item.Resp != nil {
		for _, path := range item.Resp.NewFiles {
			set.output[filepath.Clean(path)] = struct{}{}
		}
	}

	return set
}

// archive reports a path an earlier extract already unpacked. The compare is
// exact: on a case-sensitive filesystem Episode.zip is a different archive
// from episode.zip.
func (s *unpackedSet) archive(path string) bool {
	if s == nil {
		return false
	}

	_, ok := s.archives[filepath.Clean(path)]

	return ok
}

// ours reports a path an earlier extract wrote: one of its files, or anything
// inside a folder it created. An archive that was packed inside another one
// lands in the download folder this way. It is output, and it is not counted
// as delivered data or listed as a new archive.
func (s *unpackedSet) ours(path string) bool {
	if s == nil || len(s.output) == 0 {
		return false
	}

	path = filepath.Clean(path)

	for {
		if _, ok := s.output[path]; ok {
			return true
		}

		parent := filepath.Dir(path)
		if path == s.root || parent == path {
			return false
		}

		path = parent
	}
}

// accept is an xtractr Filter.Accept that lists only archives new to this item.
func (s *unpackedSet) accept(cand xtractr.ArchiveCandidate) bool {
	return !s.archive(cand.Path) && !s.ours(cand.Path)
}

// pinArchives is an xtractr Filter.Accept that lists exactly these archives.
// The queue worker searches the download again when the extract starts, which
// can be much later. A Syncthing extract must take the archives the wait
// checks looked at: nothing that arrived in between, and nothing an earlier
// extract already handled. The next pass picks up what arrived.
func pinArchives(files xtractr.ArchiveList) func(xtractr.ArchiveCandidate) bool {
	pinned := make(map[string]struct{}, files.Count())
	for _, path := range files.List() {
		pinned[filepath.Clean(path)] = struct{}{}
	}

	return func(cand xtractr.ArchiveCandidate) bool {
		_, ok := pinned[filepath.Clean(cand.Path)]
		return ok
	}
}

// keepUnpacked returns the response to store for item after next arrives.
// An item with earlier extracts keeps their files, archives, and size on
// record: delete has to remove those files too, and a restart must not lose
// them. finished is true only for an extract that succeeded.
func keepUnpacked(item *Extract, next *xtractr.Response, finished bool) *xtractr.Response {
	if next == nil || !hasUnpacked(item) || item.Resp == nil {
		return next
	}

	prev := item.Resp
	merged := *next
	merged.Size = prev.Size
	added := next.NewFiles

	if finished {
		merged.Size += next.Size
	} else {
		added = movedIntoPlace(next)
	}

	merged.NewFiles = appendUnique(prev.NewFiles, added)
	merged.Archives = mergeArchiveLists(prev.Archives, next.Archives)
	merged.Extras = mergeArchiveLists(prev.Extras, next.Extras)

	return &merged
}

// recordUnpacked adds a finished extract's archives to the item and drops
// its remnant snapshot. The next extract for this item takes a new snapshot,
// and that one holds this extract's output as files to keep.
func recordUnpacked(item *Extract, resp *xtractr.Response) {
	if !item.Syncthing {
		return
	}

	archives := resp.Archives.List()
	slices.Sort(archives)

	item.Unpacked = appendUnique(item.Unpacked, archives)
	item.PreFiles = nil
}

// movedIntoPlace is the part of a response's NewFiles that reached the
// download folder. An extract that did not finish also lists files in its
// temp folder, and xtractr deletes those.
func movedIntoPlace(resp *xtractr.Response) []string {
	if resp.Output == "" {
		return resp.NewFiles
	}

	moved := make([]string, 0, len(resp.NewFiles))

	for _, path := range resp.NewFiles {
		if !pathWithin(path, resp.Output) {
			moved = append(moved, path)
		}
	}

	return moved
}

// pathWithin reports whether path is dir or lies under it.
func pathWithin(path, dir string) bool {
	path, dir = filepath.Clean(path), filepath.Clean(dir)

	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// appendUnique returns the cleaned paths in have followed by the cleaned
// paths in add that are not listed yet.
func appendUnique(have, add []string) []string {
	out := make([]string, 0, len(have)+len(add))
	seen := make(map[string]struct{}, len(have)+len(add))

	for _, list := range [][]string{have, add} {
		for _, path := range list {
			path = filepath.Clean(path)
			if _, dup := seen[path]; dup {
				continue
			}

			seen[path] = struct{}{}

			out = append(out, path)
		}
	}

	return out
}

// mergeArchiveLists returns both lists with each archive path listed once.
func mergeArchiveLists(prev, next xtractr.ArchiveList) xtractr.ArchiveList {
	out := make(xtractr.ArchiveList, len(prev)+len(next))
	seen := make(map[string]struct{}, prev.Count()+next.Count())

	for _, list := range []xtractr.ArchiveList{prev, next} {
		for dir, paths := range list {
			for _, path := range paths {
				key := filepath.Clean(path)
				if _, dup := seen[key]; dup {
					continue
				}

				seen[key] = struct{}{}

				out[dir] = append(out[dir], path)
			}
		}
	}

	return out
}

// withoutError returns resp, or a copy with the error cleared.
func withoutError(resp *xtractr.Response) *xtractr.Response {
	if resp == nil || resp.Error == nil {
		return resp
	}

	clean := *resp
	clean.Error = nil

	return &clean
}

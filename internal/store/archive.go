package store

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// ExportZip writes the project directory as a zip archive with relative paths.
// state is the project.json to put into the archive (Encode of the in-memory
// state): the caller takes it under the project lock and the files are written
// without it, since capture files never change and report artifacts are
// written once. A file removed meanwhile (a cancelled capture import) is left out.
func (s *Store) ExportZip(id string, state []byte, w io.Writer) error {
	dir := s.ProjectDir(id)
	zw := zip.NewWriter(w)
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: stateFile, Method: zip.Deflate, Modified: time.Now()})
	if err == nil {
		_, err = fw.Write(state)
	}
	if err == nil {
		err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(dir, p)
			if err != nil || rel == "." || rel == stateFile {
				return err
			}
			name := filepath.ToSlash(rel)
			if strings.HasPrefix(path.Base(name), ".") {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil // temporary files
			}
			if d.IsDir() {
				return nil
			}
			f, err := os.Open(p)
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			defer f.Close()
			info, err := f.Stat()
			if err != nil {
				return err
			}
			hdr, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			hdr.Name = name
			hdr.Method = zip.Deflate
			if strings.HasPrefix(name, "captures/") {
				hdr.Method = zip.Store // captures compress poorly; keep export fast
			}
			fw, err := zw.CreateHeader(hdr)
			if err != nil {
				return err
			}
			_, err = io.Copy(fw, f)
			return err
		})
	}
	if err != nil {
		zw.Close()
		return err
	}
	return zw.Close()
}

// ImportLimits protect against zip bombs and path traversal.
type ImportLimits struct {
	MaxFiles      int
	MaxTotalBytes int64
	// MaxStateBytes bounds project.json, which is read into memory whole
	// (0 means DefaultMaxStateBytes).
	MaxStateBytes int64
}

// DefaultMaxStateBytes is far above any real project.json: every change of a
// project rewrites the whole file.
const DefaultMaxStateBytes = 128 << 20

// ErrBadArchive is returned for archives that are not a ProtoLab project.
var ErrBadArchive = errors.New("not a ProtoLab project archive")

// ExtractZip unpacks a project archive into a new temporary directory inside
// the projects directory and returns its path. Only project.json, captures/
// and reports/ entries are accepted.
func (s *Store) ExtractZip(r io.ReaderAt, size int64, lim ImportLimits) (string, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBadArchive, err)
	}
	if len(zr.File) > lim.MaxFiles {
		return "", fmt.Errorf("%w: too many files (%d > %d)", ErrBadArchive, len(zr.File), lim.MaxFiles)
	}
	if lim.MaxStateBytes <= 0 {
		lim.MaxStateBytes = DefaultMaxStateBytes
	}
	entries, err := acceptedEntries(zr.File, lim)
	if err != nil {
		return "", err
	}

	tmp, err := os.MkdirTemp(s.ProjectsDir(), ".import-")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(tmp)
		}
	}()

	var total int64
	for _, e := range entries {
		if err := extractFile(e.file, filepath.Join(tmp, filepath.FromSlash(e.name)), lim.MaxTotalBytes-total, &total); err != nil {
			return "", err
		}
	}
	ok = true
	return tmp, nil
}

type archiveEntry struct {
	file *zip.File
	name string // cleaned
}

// acceptedEntries selects the entries to unpack and checks them before anything
// is written: names (zip slip, duplicates, a file where a directory is needed)
// and sizes. archive/zip fails an entry that inflates to more or less than its
// declared size, so the declared sizes bound what is written; a zip bomb
// (also one of overlapping entries) is refused without touching the disk.
func acceptedEntries(files []*zip.File, lim ImportLimits) ([]archiveEntry, error) {
	var out []archiveEntry
	var total uint64
	hasState := false
	// Lower-cased: on a case-insensitive file system two such names are one file.
	isFile, isDir := map[string]bool{}, map[string]bool{}
	for _, f := range files {
		name, err := safeName(f.Name)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		if name == stateFile {
			hasState = true
			if f.UncompressedSize64 > uint64(lim.MaxStateBytes) {
				return nil, fmt.Errorf("%w: project.json is larger than %d bytes", ErrBadArchive, lim.MaxStateBytes)
			}
		} else if !strings.HasPrefix(name, "captures/") && !strings.HasPrefix(name, "reports/") {
			continue // ignore foreign files instead of failing
		}
		key := strings.ToLower(name)
		if isFile[key] || isDir[key] {
			return nil, fmt.Errorf("%w: duplicate entry %q", ErrBadArchive, f.Name)
		}
		isFile[key] = true
		for dir := path.Dir(key); dir != "."; dir = path.Dir(dir) {
			if isFile[dir] {
				return nil, fmt.Errorf("%w: entry %q is inside the file %q", ErrBadArchive, f.Name, dir)
			}
			isDir[dir] = true
		}
		total += f.UncompressedSize64
		if total > uint64(lim.MaxTotalBytes) {
			return nil, fmt.Errorf("%w: archive unpacks to more than the allowed %d bytes", ErrBadArchive, lim.MaxTotalBytes)
		}
		out = append(out, archiveEntry{file: f, name: name})
	}
	if !hasState {
		return nil, fmt.Errorf("%w: project.json is missing", ErrBadArchive)
	}
	return out, nil
}

// safeName rejects absolute paths and parent references (zip slip).
func safeName(name string) (string, error) {
	clean := path.Clean(strings.ReplaceAll(name, `\`, "/"))
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, ":") {
		return "", fmt.Errorf("%w: unsafe path %q", ErrBadArchive, name)
	}
	return clean, nil
}

func extractFile(f *zip.File, dst string, budget int64, total *int64) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadArchive, err)
	}
	defer rc.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	// Read one byte past the budget to detect an oversized entry regardless of the declared size.
	n, err := io.Copy(out, io.LimitReader(rc, budget+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > budget {
		return fmt.Errorf("%w: archive is larger than the allowed %d bytes", ErrBadArchive, budget+*total)
	}
	*total += n
	return nil
}

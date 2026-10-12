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
)

// ExportZip writes the project directory as a zip archive with relative paths.
func (s *Store) ExportZip(id string, w io.Writer) error {
	dir := s.ProjectDir(id)
	zw := zip.NewWriter(w)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil || rel == "." {
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
		info, err := d.Info()
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
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		_, err = io.Copy(fw, f)
		f.Close()
		return err
	})
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
}

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
	hasState := false
	for _, f := range zr.File {
		name, err := safeName(f.Name)
		if err != nil {
			return "", err
		}
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		if name == stateFile {
			hasState = true
		} else if !strings.HasPrefix(name, "captures/") && !strings.HasPrefix(name, "reports/") {
			continue // ignore foreign files instead of failing
		}
		if err := extractFile(f, filepath.Join(tmp, filepath.FromSlash(name)), lim.MaxTotalBytes-total, &total); err != nil {
			return "", err
		}
	}
	if !hasState {
		return "", fmt.Errorf("%w: project.json is missing", ErrBadArchive)
	}
	ok = true
	return tmp, nil
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

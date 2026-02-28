package mmindex

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"os"
	"syscall"

	"github.com/AlanKK/everythingx/internal/shared"
)

// IndexPath is where the binary flat index file is written.
const IndexPath = "/var/lib/everythingx/files.idx"

// magic identifies a valid index file.
var magic = [4]byte{'E', 'V', 'I', 'N'}

// Binary format (little-endian):
//
//	[4]  magic
//	[4]  uint32 entry count
//	repeated per entry:
//	  [2]  uint16 filename length
//	  [N]  filename bytes
//	  [2]  uint16 fullpath length
//	  [N]  fullpath bytes
//	  [1]  uint8  object_type

// Build writes the binary index to IndexPath.
func Build(db *sql.DB) error {
	return BuildAt(db, IndexPath)
}

// BuildAt reads all rows from db sorted by filename and atomically writes the
// binary index to path. The old file remains valid for existing mappings
// because the rename only replaces the directory entry; the old inode persists
// until all mappings are closed.
func BuildAt(db *sql.DB, path string) error {
	rows, err := db.Query("SELECT filename, fullpath, object_type FROM files ORDER BY filename ASC")
	if err != nil {
		return err
	}
	defer rows.Close()

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		f.Close()
		if !success {
			os.Remove(tmp)
		}
	}()

	w := bufio.NewWriterSize(f, 1<<20) // 1 MB write buffer

	// Write placeholder header; count is patched after all rows are written.
	header := make([]byte, 8)
	copy(header[:4], magic[:])
	if _, err := w.Write(header); err != nil {
		return err
	}

	var count uint32
	for rows.Next() {
		var filename, fullpath string
		var objType shared.ObjectType
		if err := rows.Scan(&filename, &fullpath, &objType); err != nil {
			return err
		}
		fn := []byte(filename)
		fp := []byte(fullpath)

		if err := binary.Write(w, binary.LittleEndian, uint16(len(fn))); err != nil {
			return err
		}
		if _, err := w.Write(fn); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, uint16(len(fp))); err != nil {
			return err
		}
		if _, err := w.Write(fp); err != nil {
			return err
		}
		if err := w.WriteByte(byte(objType)); err != nil {
			return err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// Patch entry count into header.
	binary.LittleEndian.PutUint32(header[4:], count)
	if _, err := f.WriteAt(header, 0); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	f.Close()

	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	success = true
	return nil
}

// Index is a memory-mapped view of the binary flat index file.
// The OS manages which pages are resident in RAM and can evict them under
// memory pressure, unlike a Go-heap slice which pins all data in the heap.
type Index struct {
	data []byte
}

// Open memory-maps IndexPath. The caller must call Close when done.
func Open() (*Index, error) {
	return OpenAt(IndexPath)
}

// OpenAt memory-maps the index file at path. The caller must call Close when done.
func OpenAt(path string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Size() < 8 {
		return nil, fmt.Errorf("mmindex: file too small")
	}

	data, err := syscall.Mmap(int(f.Fd()), 0, int(fi.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mmindex: mmap: %w", err)
	}
	if !bytes.Equal(data[:4], magic[:]) {
		syscall.Munmap(data) //nolint:errcheck
		return nil, fmt.Errorf("mmindex: invalid magic")
	}
	return &Index{data: data}, nil
}

// Close unmaps the index from memory.
func (idx *Index) Close() error {
	return syscall.Munmap(idx.data)
}

// Search returns up to limit results whose filename contains term.
// It scans the mmap'd bytes directly using bytes.Contains to avoid
// per-entry heap allocations.
func (idx *Index) Search(term string, limit int) []*shared.SearchResult {
	termBytes := []byte(term)
	count := binary.LittleEndian.Uint32(idx.data[4:8])
	d := idx.data
	pos := 8

	var results []*shared.SearchResult
	for i := uint32(0); i < count; i++ {
		if pos+2 > len(d) {
			break
		}
		fnLen := int(binary.LittleEndian.Uint16(d[pos:]))
		pos += 2
		if pos+fnLen > len(d) {
			break
		}
		fn := d[pos : pos+fnLen]
		pos += fnLen

		if pos+2 > len(d) {
			break
		}
		fpLen := int(binary.LittleEndian.Uint16(d[pos:]))
		pos += 2
		if pos+fpLen > len(d) {
			break
		}
		fp := d[pos : pos+fpLen]
		pos += fpLen

		if pos >= len(d) {
			break
		}
		objType := shared.ObjectType(d[pos])
		pos++

		if bytes.Contains(fn, termBytes) {
			results = append(results, &shared.SearchResult{
				Fullpath:   string(fp),
				ObjectType: objType,
			})
			if len(results) >= limit {
				break
			}
		}
	}
	return results
}

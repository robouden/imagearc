// Package store is a pure-Go SQLite (modernc) index of captioned/tagged photos
// with FTS5 full-text search, backing the in-app library and dashboard.
package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Photo is one indexed image.
type Photo struct {
	Path     string   `json:"path"`
	Filename string   `json:"filename"`
	Caption  string   `json:"caption"`
	Keywords string   `json:"keywords"` // comma-separated
	Byline   string   `json:"byline"`
	Location string   `json:"location"`
	Date     string   `json:"date"`
	Lat      *float64 `json:"lat,omitempty"`
	Lon      *float64 `json:"lon,omitempty"`
}

// ReaderVersion is bumped whenever metadata extraction changes, so existing
// rows are re-read on the next scan even if their mtime is unchanged.
// v2: added EXIF DateTimeOriginal + GPS lat/lon.
// v3: offline reverse-geocoded "City, Country" location from GPS.
const ReaderVersion = 3

// Store wraps the SQLite database.
type Store struct{ db *sql.DB }

// DefaultPath returns ~/.config/imagearc/library.db (honoring XDG_CONFIG_HOME).
func DefaultPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "imagearc", "library.db")
}

// Open opens (creating if needed) the library database and applies the schema.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS photos(
  id INTEGER PRIMARY KEY,
  path TEXT UNIQUE NOT NULL,
  filename TEXT, caption TEXT, keywords TEXT, byline TEXT, location TEXT, date TEXT,
  mtime INTEGER, indexed_at INTEGER
);
CREATE TABLE IF NOT EXISTS sources(
  path TEXT PRIMARY KEY,
  recurse INTEGER
);
CREATE VIRTUAL TABLE IF NOT EXISTS photos_fts USING fts5(
  caption, keywords, byline, location, content='photos', content_rowid='id'
);
CREATE TRIGGER IF NOT EXISTS photos_ai AFTER INSERT ON photos BEGIN
  INSERT INTO photos_fts(rowid, caption, keywords, byline, location)
  VALUES (new.id, new.caption, new.keywords, new.byline, new.location);
END;
CREATE TRIGGER IF NOT EXISTS photos_ad AFTER DELETE ON photos BEGIN
  INSERT INTO photos_fts(photos_fts, rowid, caption, keywords, byline, location)
  VALUES('delete', old.id, old.caption, old.keywords, old.byline, old.location);
END;
CREATE TRIGGER IF NOT EXISTS photos_au AFTER UPDATE ON photos BEGIN
  INSERT INTO photos_fts(photos_fts, rowid, caption, keywords, byline, location)
  VALUES('delete', old.id, old.caption, old.keywords, old.byline, old.location);
  INSERT INTO photos_fts(rowid, caption, keywords, byline, location)
  VALUES (new.id, new.caption, new.keywords, new.byline, new.location);
END;`)
	if err != nil {
		return err
	}
	// Add columns to pre-existing databases; ignore "duplicate column" errors.
	s.db.Exec("ALTER TABLE photos ADD COLUMN lat REAL")
	s.db.Exec("ALTER TABLE photos ADD COLUMN lon REAL")
	s.db.Exec("ALTER TABLE photos ADD COLUMN reader_version INTEGER DEFAULT 0")
	return nil
}

// Upsert inserts or updates a photo by path.
func (s *Store) Upsert(p Photo) error {
	mtime := int64(0)
	if fi, err := os.Stat(p.Path); err == nil {
		mtime = fi.ModTime().Unix()
	}
	if p.Filename == "" {
		p.Filename = filepath.Base(p.Path)
	}
	_, err := s.db.Exec(`
INSERT INTO photos(path, filename, caption, keywords, byline, location, date, lat, lon, mtime, indexed_at, reader_version)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(path) DO UPDATE SET
  filename=excluded.filename, caption=excluded.caption, keywords=excluded.keywords,
  byline=excluded.byline, location=excluded.location, date=excluded.date,
  lat=excluded.lat, lon=excluded.lon, mtime=excluded.mtime, indexed_at=excluded.indexed_at,
  reader_version=excluded.reader_version`,
		p.Path, p.Filename, p.Caption, p.Keywords, p.Byline, p.Location, p.Date,
		p.Lat, p.Lon, mtime, time.Now().Unix(), ReaderVersion)
	return err
}

// IsFresh reports whether path is already indexed with the given mtime AND the
// current ReaderVersion, i.e. it can be skipped during an incremental scan.
func (s *Store) IsFresh(path string, mtime int64) bool {
	var mt int64
	var ver int
	err := s.db.QueryRow("SELECT mtime, reader_version FROM photos WHERE path = ?", path).Scan(&mt, &ver)
	if err != nil {
		return false
	}
	return mt == mtime && ver == ReaderVersion
}

// PathsUnder returns all indexed paths equal to root or beneath it.
func (s *Store) PathsUnder(root string) ([]string, error) {
	rows, err := s.db.Query("SELECT path FROM photos WHERE path = ? OR path LIKE ?", root, root+"/%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Delete removes a photo from the index by path.
func (s *Store) Delete(path string) error {
	_, err := s.db.Exec("DELETE FROM photos WHERE path = ?", path)
	return err
}

// Source is a remembered indexed folder.
type Source struct {
	Path    string `json:"path"`
	Recurse bool   `json:"recurse"`
}

// AddSource records (or updates) a folder as an index source.
func (s *Store) AddSource(path string, recurse bool) error {
	_, err := s.db.Exec(
		"INSERT INTO sources(path, recurse) VALUES(?, ?) ON CONFLICT(path) DO UPDATE SET recurse=excluded.recurse",
		path, boolToInt(recurse))
	return err
}

// Sources returns all remembered index sources.
func (s *Store) Sources() ([]Source, error) {
	rows, err := s.db.Query("SELECT path, recurse FROM sources ORDER BY path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		var src Source
		var rec int
		if err := rows.Scan(&src.Path, &rec); err != nil {
			return nil, err
		}
		src.Recurse = rec != 0
		out = append(out, src)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Query holds search parameters.
type Query struct {
	Text     string // FTS full-text query
	Keyword  string // exact-ish keyword filter (LIKE)
	Location string
	Byline   string
	Limit    int
	Offset   int
}

// ftsExpr turns free text into a safe FTS5 prefix-AND expression.
func ftsExpr(text string) string {
	var terms []string
	for _, t := range strings.Fields(text) {
		t = strings.ReplaceAll(t, `"`, "")
		if t != "" {
			terms = append(terms, `"`+t+`"*`)
		}
	}
	return strings.Join(terms, " ")
}

// Search returns matching photos and the total match count (ignoring limit/offset).
func (s *Store) Search(q Query) ([]Photo, int, error) {
	var where []string
	var args []any
	if expr := ftsExpr(q.Text); expr != "" {
		where = append(where, "p.id IN (SELECT rowid FROM photos_fts WHERE photos_fts MATCH ?)")
		args = append(args, expr)
	}
	if q.Keyword != "" {
		where = append(where, "p.keywords LIKE ?")
		args = append(args, "%"+q.Keyword+"%")
	}
	if q.Location != "" {
		where = append(where, "p.location LIKE ?")
		args = append(args, "%"+q.Location+"%")
	}
	if q.Byline != "" {
		where = append(where, "p.byline LIKE ?")
		args = append(args, "%"+q.Byline+"%")
	}
	clause := ""
	if len(where) > 0 {
		clause = "WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM photos p "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		"SELECT p.path, p.filename, p.caption, p.keywords, p.byline, p.location, p.date, p.lat, p.lon FROM photos p "+
			clause+" ORDER BY p.date DESC, p.filename ASC LIMIT ? OFFSET ?",
		append(args, limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out, err := scanPhotos(rows)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Geo returns every photo that has GPS coordinates, for the map view.
func (s *Store) Geo() ([]Photo, error) {
	rows, err := s.db.Query(
		"SELECT path, filename, caption, keywords, byline, location, date, lat, lon " +
			"FROM photos WHERE lat IS NOT NULL AND lon IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPhotos(rows)
}

func scanPhotos(rows *sql.Rows) ([]Photo, error) {
	var out []Photo
	for rows.Next() {
		var p Photo
		var lat, lon sql.NullFloat64
		if err := rows.Scan(&p.Path, &p.Filename, &p.Caption, &p.Keywords,
			&p.Byline, &p.Location, &p.Date, &lat, &lon); err != nil {
			return nil, err
		}
		if lat.Valid {
			p.Lat = &lat.Float64
		}
		if lon.Valid {
			p.Lon = &lon.Float64
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// KV is a label/count pair for dashboard aggregates.
type KV struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// Stats holds dashboard aggregates.
type Stats struct {
	Total       int  `json:"total"`
	Captioned   int  `json:"captioned"`
	Geotagged   int  `json:"geotagged"`
	TopKeywords []KV `json:"topKeywords"`
	Locations   []KV `json:"locations"`
	Bylines     []KV `json:"bylines"`
	Years       []KV `json:"years"`
}

// Stats computes dashboard aggregates. Keyword splitting is done in Go.
func (s *Store) Stats() (Stats, error) {
	var st Stats
	if err := s.db.QueryRow(
		"SELECT COUNT(*), COUNT(NULLIF(TRIM(caption),'')), COUNT(lat) FROM photos").
		Scan(&st.Total, &st.Captioned, &st.Geotagged); err != nil {
		return st, err
	}
	st.Locations, _ = s.groupCount("location")
	st.Bylines, _ = s.groupCount("byline")
	st.Years, _ = s.yearCounts()

	rows, err := s.db.Query("SELECT keywords FROM photos WHERE TRIM(keywords) <> ''")
	if err != nil {
		return st, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var kw string
		if err := rows.Scan(&kw); err != nil {
			return st, err
		}
		for _, k := range strings.Split(kw, ",") {
			k = strings.TrimSpace(k)
			if k != "" {
				counts[strings.ToLower(k)]++
			}
		}
	}
	st.TopKeywords = topN(counts, 20)
	return st, nil
}

// yearCounts groups photos by the year prefix of their date, newest first.
func (s *Store) yearCounts() ([]KV, error) {
	rows, err := s.db.Query(
		"SELECT substr(date,1,4) y, COUNT(*) c FROM photos WHERE length(date) >= 4 " +
			"GROUP BY y ORDER BY y DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KV
	for rows.Next() {
		var kv KV
		if err := rows.Scan(&kv.Label, &kv.Count); err != nil {
			return nil, err
		}
		out = append(out, kv)
	}
	return out, rows.Err()
}

func (s *Store) groupCount(col string) ([]KV, error) {
	rows, err := s.db.Query("SELECT " + col + ", COUNT(*) c FROM photos WHERE TRIM(" + col +
		") <> '' GROUP BY " + col + " ORDER BY c DESC LIMIT 20")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KV
	for rows.Next() {
		var kv KV
		if err := rows.Scan(&kv.Label, &kv.Count); err != nil {
			return nil, err
		}
		out = append(out, kv)
	}
	return out, rows.Err()
}

func topN(counts map[string]int, n int) []KV {
	out := make([]KV, 0, len(counts))
	for k, v := range counts {
		out = append(out, KV{Label: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

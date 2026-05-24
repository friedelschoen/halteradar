/*
Copyright (C) 2026 Friedel Schön

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package main

import (
	"archive/zip"
	"database/sql"
	"encoding/csv"
	"errors"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/friedelschoen/halteradar/gtfs_import/task"
	_ "github.com/lib/pq"
)

const staleAfter = 12 * time.Hour

type Server struct {
	archivePath string

	db      *sql.DB
	feedRef int64

	tmpdir string

	agencies,
	routes,
	trips,
	services,
	shapes,
	stops map[string]struct{}

	stopsCollected bool
}

func (server *Server) activeFeedIsStale() (bool, error) {
	var importedAt time.Time

	err := server.db.QueryRow(`
		SELECT imported_at
		FROM gtfs_feeds
		WHERE active = true
		ORDER BY imported_at DESC
		LIMIT 1
	`).Scan(&importedAt)

	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return time.Since(importedAt) >= staleAfter, nil
}

func (server *Server) insertFeed(row map[string]string) (int64, bool, error) {
	var id int64
	var inserted bool

	err := server.db.QueryRow(`
		INSERT INTO gtfs_feeds (
			feed_publisher_name,
			feed_id,
			feed_publisher_url,
			feed_lang,
			feed_start_date,
			feed_end_date,
			feed_version,
			active
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, false)
		ON CONFLICT (feed_id, feed_version)
		DO UPDATE SET imported_at = now()
		RETURNING id, xmax = 0
	`,
		nullString(row["feed_publisher_name"]),
		nullString(row["feed_id"]),
		nullString(row["feed_publisher_url"]),
		nullString(row["feed_lang"]),
		parseGTFSDate(row["feed_start_date"]),
		parseGTFSDate(row["feed_end_date"]),
		nullString(row["feed_version"]),
	).Scan(&id, &inserted)

	return id, inserted, err
}

func readFirstRow(rc io.Reader) (map[string]string, error) {
	r := csv.NewReader(rc)
	r.FieldsPerRecord = -1

	keys, err := r.Read()
	if err != nil {
		return nil, err
	}
	values, err := r.Read()
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(keys))
	for i, key := range keys {
		if i < len(values) {
			result[key] = values[i]
		} else {
			result[key] = ""
		}
	}
	return result, nil
}

func (s *Server) iterCSV(progress func(float64), filename string, fn func(map[string]string) error) error {
	rc, err := os.Open(filepath.Join(s.tmpdir, filename))
	if err != nil {
		return err
	}
	defer rc.Close()

	var totalSize int64
	if fileinfo, err := rc.Stat(); err == nil {
		totalSize = fileinfo.Size()
	}

	r := csv.NewReader(rc)
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return err
	}

	var total int

	row := make(map[string]string, len(header))
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		total++

		off := r.InputOffset()
		progress(float64(off) / float64(totalSize))

		for k := range row {
			row[k] = ""
		}
		for i, h := range header {
			if i < len(rec) {
				row[h] = rec[i]
			}
		}

		if err := fn(row); err != nil {
			return err
		}
	}
	return err
}

func (s *Server) insertCSV(progress func(float64), filename string, query string, fn func(*Server, map[string]string) []any) error {
	var total, skipped int

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	err = s.iterCSV(progress, filename, func(row map[string]string) error {
		total++
		insert := fn(s, row)
		if len(insert) == 0 {
			skipped++
			return nil
		}
		_, err := stmt.Exec(insert...)
		return err
	})
	if err != nil {
		return err
	}

	progress(-1)
	_, err = stmt.Exec()
	if err != nil {
		return err
	}

	return tx.Commit()
}

func parseGTFSDate(s string) any {
	if s == "" {
		return nil
	}

	t, err := time.Parse("20060102", s)
	if err != nil {
		return nil
	}

	return t
}

func parseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

func parseNullableInt(s string) any {
	if s == "" {
		return nil
	}

	i, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}

	return i
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func parseNullableFloat(s string) any {
	if s == "" {
		return nil
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}

	return f
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func unpackFile(dest string, f *zip.File) error {
	instream, err := f.Open()
	if err != nil {
		return err
	}
	defer instream.Close()

	destpath := filepath.Join(dest, f.Name)
	outstream, err := os.Create(destpath)
	if err != nil {
		return err
	}
	defer outstream.Close()

	_, err = io.Copy(outstream, instream)
	return err
}

type MetaTask []string

func (t MetaTask) NeedsRun(server *Server) (bool, error)                { return true, nil }
func (t MetaTask) Group() string                                        { return "" }
func (t MetaTask) Execute(server *Server, progress func(float64)) error { return nil }
func (t MetaTask) Cleanup(*Server) error                                { return nil }
func (t MetaTask) Dependencies() []string                               { return t }

type Task struct {
	needsRun func(*Server) (bool, error)
	execute  func(server *Server, progress func(float64)) error
	cleanup  func(*Server) error

	deps  []string
	group string
}

func (t Task) NeedsRun(server *Server) (bool, error) {
	if t.needsRun != nil {
		return t.needsRun(server)
	}
	return true, nil
}

func (t Task) Execute(server *Server, progress func(float64)) error {
	return t.execute(server, progress)
}

func (t Task) Cleanup(server *Server) error {
	if t.cleanup != nil {
		return t.cleanup(server)
	}
	return nil
}

func (t Task) Dependencies() []string {
	return t.deps
}

func (t Task) Group() string {
	return t.group
}

var tasks = map[string]task.Task[*Server]{
	"feed_ref": Task{
		deps: []string{"archive"},
		needsRun: func(s *Server) (bool, error) {
			return s.feedRef == 0, nil
			// {
			// 	return false, nil
			// }
			// needsUpdate, err := s.activeFeedIsStale()
			// if err != nil {
			// 	return false, err
			// }
			// return needsUpdate, nil
		},
		execute: func(server *Server, progress func(float64)) error {
			file, err := os.Open(filepath.Join(server.tmpdir, "feed_info.txt"))
			if err != nil {
				return err
			}
			feedInfo, err := readFirstRow(file)
			if err != nil {
				return err
			}
			file.Close()

			feedRef, _, err := server.insertFeed(feedInfo)
			if err != nil {
				return err
			}
			server.feedRef = feedRef
			return nil
		},
	},
	"tmpdir": Task{
		execute: func(server *Server, progress func(float64)) error {
			server.tmpdir = filepath.Join(os.TempDir(), "gtfs_updater")
			err := os.Mkdir(server.tmpdir, 0775)
			if errors.Is(err, os.ErrExist) {
				return nil
			}
			return err
		},
		cleanup: func(s *Server) error {
			return os.RemoveAll(s.tmpdir)
		},
	},
	"archive": Task{
		deps: []string{"tmpdir"},
		execute: func(server *Server, progress func(float64)) error {
			buf, err := os.Open(server.archivePath)
			if err != nil {
				return err
			}
			defer buf.Close()

			size, err := buf.Seek(0, io.SeekEnd)
			if err != nil {
				return err
			}

			zr, err := zip.NewReader(buf, size)
			if err != nil {
				return err
			}

			for i, f := range zr.File {
				progress(float64(i) / float64(len(zr.File)))
				if err := unpackFile(server.tmpdir, f); err != nil {
					return err
				}
			}
			return nil
		},
	},
	"activate": Task{
		deps: []string{"import_all", "calc_all"},
		execute: func(server *Server, progress func(float64)) error {
			tx, err := server.db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback()
			if _, err := tx.Exec(`UPDATE gtfs_feeds SET active = false`); err != nil {
				return err
			}
			_, err = tx.Exec(`
		UPDATE gtfs_feeds
		SET active = true, imported_at = now()
		WHERE id = $1
	`, server.feedRef)

			return tx.Commit()
		},
	},

	"collect_routes":     collectRoutesTask,
	"collect_trips":      collectTripsTask,
	"collect_stop_times": collectStopTimesTask,
	"collect_stops":      collectStopsTask,
	"collect_all":        MetaTask{"collect_routes", "collect_trips", "collect_stop_times", "collect_stops"},

	"import_agencies":       importAgenciesTask,
	"import_routes":         importRoutesTask,
	"import_trips":          importTripsTask,
	"import_calendar_dates": importCalendarDatesTask,
	"import_stops":          importStopsTask,
	"import_stop_times":     importStopTimesTask,
	"import_shapes":         importShapesTask,
	"import_all": MetaTask{
		"import_agencies",
		"import_routes",
		"import_trips",
		"import_calendar_dates",
		"import_stops",
		"import_stop_times",
		"import_shapes",
	},

	"clear_trip_bounds":   clearTripBounds,
	"clear_stop_events":   clearStopEvents,
	"calc_trip_bounds":    calculateTripBounds,
	"calc_stop_events":    calculateStopEvents,
	"calc_rtt_sequence":   calculateRTTSequence,
	"calc_missing_shapes": calculateShapes,
	"calc_all": MetaTask{
		"calc_trip_bounds",
		"calc_stop_events",
		"calc_rtt_sequence",
		"calc_missing_shapes",
	},
}

func main() {
	var (
		dbpath   = flag.String("db", os.Getenv("POSTGRES"), "path to postgres")
		archive  = flag.String("archive", "", "Path to GTFS archive")
		feedRef  = flag.Int64("feedref", 0, "Feed Ref")
		agencies = flag.String("agencies", "QBUZZ", "path to postgres, comma-delimited")
		workers  = flag.Int("jobs", runtime.NumCPU(), "parallel workers")
		force    = flag.Bool("force", false, "run all tasks")
	)

	flag.Parse()

	if *dbpath == "" {
		log.Fatal("missing $POSTGRES env")
	}

	if *archive == "" && *feedRef == 0 {
		log.Fatal("either -archive or -feedref")
	}

	db, err := sql.Open("postgres", *dbpath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	var s Server
	s.db = db
	s.archivePath = *archive
	s.feedRef = *feedRef
	s.agencies = map[string]struct{}{}

	for a := range strings.SplitSeq(*agencies, ",") {
		if a == "" {
			continue
		}

		s.agencies[a] = struct{}{}
	}

	var g task.TaskGraph[*Server]
	if err := g.Add(tasks, flag.Args()...); err != nil {
		log.Fatalln(err)
	}

	var r task.TaskRunner[*Server]
	r.G = g
	r.State = &s
	r.Workers = *workers
	r.RunAll = *force

	if err := r.Execute(); err != nil {
		log.Fatalln(err)
	}
}

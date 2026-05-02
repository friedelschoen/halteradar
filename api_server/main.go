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
	"database/sql"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
)

const defaultStopID = "stoparea:449933"

type Server struct {
	db *sql.DB
}

func main() {
	dburl := os.Getenv("POSTGRES")
	if dburl == "" {
		log.Fatal("missing POSTGRES env")
	}

	db, err := sql.Open("postgres", dburl)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	s := &Server{db: db}

	http.HandleFunc("/api/departures", s.departures)
	http.HandleFunc("/api/buffer", s.stationBuffer)
	http.HandleFunc("/api/stop_info", s.stopinfo)
	http.HandleFunc("/api/stop_query", s.stopQuery)
	http.HandleFunc("/", http.NotFound)

	log.Println("api_server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

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
	_ "embed"
	"encoding/json"
	"net/http"

	"github.com/dylandreimerink/go-rijksdriehoek"
)

type Vehicle struct {
	StopID   *string `json:"stop_id"`
	StopName *string `json:"stop_name"`

	Line     *string `json:"line"`
	FGColor  *string `json:"fg_color"`
	BGColor  *string `json:"bg_color"`
	RouteID  *string `json:"route_id"`
	TripID   *string `json:"trip_id"`
	Headsign *string `json:"headsign"`

	Status       *string `json:"status"`
	DelayMinutes int     `json:"delay_minutes"`
	BlockCode    *int    `json:"block_code"`
	Lat          float64 `json:"latitude"`
	Lon          float64 `json:"longitude"`
}

type Departure struct {
	Line          string   `json:"line"`
	FGColor       *string  `json:"fg_color"`
	BGColor       *string  `json:"bg_color"`
	RouteID       string   `json:"route_id"`
	TripID        string   `json:"trip_id"`
	Headsign      string   `json:"headsign"`
	Platform      *string  `json:"platform"`
	ScheduledTime int64    `json:"scheduled_time"`
	RealtimeTime  int64    `json:"realtime_time"`
	DelayMinutes  int      `json:"delay_minutes"`
	BlockCode     *int     `json:"blockcode"`
	Terminal      bool     `json:"terminal"`
	VehicleNumber *int     `json:"id"`
	Lat           float64  `json:"latitude"`
	Lon           float64  `json:"longitude"`
	Status        *string  `json:"status"`
	Vehicle       *Vehicle `json:"vehicle,omitempty"`
}

//go:embed sql/departure.sql
var departureSQL string

func (s *Server) departures(w http.ResponseWriter, r *http.Request) {
	stopID := r.URL.Query().Get("stop")
	if stopID == "" {
		stopID = defaultStopID
	}

	rows, err := s.db.Query(departureSQL, stopID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var out []Departure

	for rows.Next() {
		var (
			d                           Departure
			veh                         Vehicle
			punctuality, vehPunctuality int
			rdx, rdy, vehRdx, vehRdy    *int
		)

		if err := rows.Scan(
			&d.RouteID,
			&d.Line,
			&d.BGColor,
			&d.FGColor,
			&d.TripID,
			&d.Headsign,
			&d.Platform,
			&d.ScheduledTime,
			&d.Terminal,
			&d.Status,
			&punctuality,
			&d.VehicleNumber,
			&d.BlockCode,
			&rdx,
			&rdy,

			&veh.StopID,
			&veh.StopName,
			&veh.RouteID,
			&veh.Line,
			&veh.FGColor,
			&veh.BGColor,
			&veh.TripID,
			&veh.Headsign,
			&veh.Status,
			&vehPunctuality,
			&veh.BlockCode,
			&vehRdx,
			&vehRdy,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		d.DelayMinutes = punctuality / 60
		veh.DelayMinutes = vehPunctuality / 60
		d.RealtimeTime = d.ScheduledTime + int64(punctuality) // both are seconds

		if rdx != nil && rdy != nil {
			d.Lat, d.Lon = rijksdriehoek.RDtoWGS84(float64(*rdx), float64(*rdy))
		}
		if vehRdx != nil && vehRdy != nil {
			veh.Lat, veh.Lon = rijksdriehoek.RDtoWGS84(float64(*vehRdx), float64(*vehRdy))
		}

		if d.VehicleNumber != nil {
			d.Vehicle = &veh
		}

		out = append(out, d)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"source": "https://github.com/friedelschoen/departures",
		"departures": out,
	})
}

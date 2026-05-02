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
)

type StopInfo struct {
	StopID             string   `json:"stop_id"`
	StopCode           *string  `json:"stop_code"`
	StopName           *string  `json:"stop_name"`
	StopLat            *float64 `json:"stop_lat"`
	StopLon            *float64 `json:"stop_lon"`
	LocationType       *int     `json:"location_type"`
	ParentStation      *string  `json:"parent_station"`
	StopTimezone       *string  `json:"stop_timezone"`
	WheelchairBoarding *string  `json:"wheelchair_boarding"`
	PlatformCode       *string  `json:"platform_code"`
	ZoneID             *string  `json:"zone_id"`
}

//go:embed sql/stop_info.sql
var stopinfoSQL string

func (s *Server) stopinfo(w http.ResponseWriter, r *http.Request) {
	stopID := r.URL.Query().Get("stop")
	if stopID == "" {
		stopID = defaultStopID
	}

	rows, err := s.db.Query(stopinfoSQL, stopID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	if !rows.Next() {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "stop not found"})
	}

	var stop StopInfo
	if err := rows.Scan(
		&stop.StopID,
		&stop.StopCode,
		&stop.StopName,
		&stop.StopLat,
		&stop.StopLon,
		&stop.LocationType,
		&stop.ParentStation,
		&stop.StopTimezone,
		&stop.WheelchairBoarding,
		&stop.PlatformCode,
		&stop.ZoneID,
	); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"source": "https://github.com/friedelschoen/departures",
		"stop": stop,
	})
}

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

type ImportTask struct {
	tableName    string
	filename     string
	query        string
	deps         []string
	rowProcessor func(server *Server, row map[string]string) []any
}

func (t ImportTask) NeedsRun(server *Server) (bool, error) {
	query := "SELECT EXISTS(SELECT 1 FROM " + t.tableName + " WHERE feed_ref = $1)"

	var exists bool
	err := server.db.QueryRow(query, server.feedRef).Scan(&exists)
	if err != nil {
		return true, err
	}
	return !exists, nil
}

func (t ImportTask) Group() string {
	return t.tableName
}

func (t ImportTask) Execute(server *Server, progress func(float64)) error {
	return server.insertCSV(progress, t.filename, t.query, t.rowProcessor)
}

func (t ImportTask) Cleanup(*Server) error { return nil }

func (t ImportTask) Dependencies() []string {
	return append([]string{"feed_ref", "archive"}, t.deps...)
}

var importAgenciesTask = ImportTask{
	tableName: "gtfs_agency",
	filename:  "agency.txt",
	query: `
			COPY gtfs_agency (
				feed_ref,
				agency_id,
				agency_name,
				agency_url,
				agency_timezone,
				agency_phone
			) FROM STDIN
		`,
	rowProcessor: func(server *Server, row map[string]string) []any {
		agencyID := row["agency_id"]
		if _, ok := server.agencies[agencyID]; !ok {
			return nil
		}
		return []any{
			server.feedRef,
			row["agency_id"],
			row["agency_name"],
			nullString(row["agency_url"]),
			nullString(row["agency_timezone"]),
			nullString(row["agency_phone"]),
		}
	},
}

var importCalendarDatesTask = ImportTask{
	tableName: "gtfs_calendar_dates",
	filename:  "calendar_dates.txt",
	deps:      []string{"collect_trips"},
	query: `
			COPY gtfs_calendar_dates (
				feed_ref,
				service_id,
				date,
				exception_type
			) FROM STDIN
		`,
	rowProcessor: func(server *Server, row map[string]string) []any {
		serviceID := row["service_id"]
		if _, ok := server.services[serviceID]; !ok {
			return nil
		}
		return []any{
			server.feedRef,
			row["service_id"],
			parseGTFSDate(row["date"]),
			parseInt(row["exception_type"]),
		}
	},
}

var importRoutesTask = ImportTask{
	tableName: "gtfs_routes",
	filename:  "routes.txt",
	deps:      []string{"collect_routes", "import_agencies"},
	query: `
			COPY gtfs_routes (
				feed_ref,
				route_id,
				agency_id,
				route_short_name,
				route_long_name,
				route_desc,
				route_type,
				route_color,
				route_text_color,
				route_url
			) FROM STDIN
		`,
	rowProcessor: func(server *Server, row map[string]string) []any {
		routeID := row["route_id"]
		if _, ok := server.routes[routeID]; !ok {
			return nil
		}

		return []any{
			server.feedRef,
			routeID,
			row["agency_id"],
			nullString(row["route_short_name"]),
			nullString(row["route_long_name"]),
			nullString(row["route_desc"]),
			nullString(row["route_type"]),
			nullString(row["route_color"]),
			nullString(row["route_text_color"]),
			nullString(row["route_url"]),
		}
	},
}

var importShapesTask = ImportTask{
	tableName: "gtfs_shapes",
	filename:  "shapes.txt",
	deps:      []string{"collect_trips"},
	query: `
			COPY gtfs_shapes (
				feed_ref,
				shape_id,
				shape_pt_sequence,
				shape_pt_lat,
				shape_pt_lon,
				shape_dist_traveled
			) FROM STDIN
		`,
	rowProcessor: func(server *Server, row map[string]string) []any {
		shapeID := row["shape_id"]
		if _, ok := server.shapes[shapeID]; !ok {
			return nil
		}
		return []any{
			server.feedRef,
			row["shape_id"],
			parseInt(row["shape_pt_sequence"]),
			parseFloat(row["shape_pt_lat"]),
			parseFloat(row["shape_pt_lon"]),
			parseNullableFloat(row["shape_dist_traveled"]),
		}
	},
}

var importStopsTask = ImportTask{
	tableName: "gtfs_stops",
	filename:  "stops.txt",
	deps:      []string{"collect_stops"},
	query: `
			COPY gtfs_stops (
				feed_ref,
				stop_id,
				stop_code,
				stop_name,
				stop_lat,
				stop_lon,
				location_type,
				parent_station,
				stop_timezone,
				wheelchair_boarding,
				zone_id
			) FROM STDIN
		`,
	rowProcessor: func(server *Server, row map[string]string) []any {
		stopID := row["stop_id"]
		if tr, ok := server.stops[stopID]; !ok || tr[0] != "" {
			return nil
		}
		return []any{
			server.feedRef,
			stopID,
			nullString(row["stop_code"]),
			nullString(row["stop_name"]),
			parseNullableFloat(row["stop_lat"]),
			parseNullableFloat(row["stop_lon"]),
			parseNullableInt(row["location_type"]),
			nullString(row["parent_station"]),
			nullString(row["stop_timezone"]),
			parseNullableInt(row["wheelchair_boarding"]),
			nullString(row["zone_id"]),
		}
	},
}

var importTripsTask = ImportTask{
	tableName: "gtfs_trips",
	filename:  "trips.txt",
	deps:      []string{"collect_routes", "import_routes"},
	query: `
			COPY gtfs_trips (
				feed_ref,
				route_id,
				service_id,
				trip_id,
				realtime_trip_id,
				trip_headsign,
				trip_short_name,
				trip_long_name,
				direction_id,
				block_id,
				shape_id,
				wheelchair_accessible,
				bikes_allowed
			) FROM STDIN
		`,

	rowProcessor: func(server *Server, row map[string]string) []any {
		routeID := row["route_id"]
		if _, ok := server.routes[routeID]; !ok {
			return nil
		}
		return []any{
			server.feedRef,
			routeID,
			row["service_id"],
			row["trip_id"],
			nullString(row["realtime_trip_id"]),
			nullString(row["trip_headsign"]),
			nullString(row["trip_short_name"]),
			nullString(row["trip_long_name"]),
			parseNullableInt(row["direction_id"]),
			nullString(row["block_id"]),
			nullString(row["shape_id"]),
			parseNullableInt(row["wheelchair_accessible"]),
			parseNullableInt(row["bikes_allowed"]),
		}
	},
}

var importStopTimesTask = ImportTask{
	tableName: "gtfs_stop_times",
	filename:  "stop_times.txt",
	deps:      []string{"collect_stop_times", "collect_trips", "collect_stops", "import_trips", "import_stops"},
	query: `
			COPY gtfs_stop_times (
				feed_ref,
				trip_id,
				stop_sequence,
				stop_id,
				platform_code,
				stop_headsign,
				arrival_time,
				departure_time,
				pickup_type,
				drop_off_type,
				timepoint,
				shape_dist_traveled,
				fare_units_traveled
			) FROM STDIN
		`,
	rowProcessor: func(server *Server, row map[string]string) []any {
		tripID := row["trip_id"]
		if _, ok := server.trips[tripID]; !ok {
			return nil
		}
		stopID := row["stop_id"]
		platformCode := ""
		if tr, ok := server.stops[stopID]; ok && tr[0] != "" {
			stopID = tr[0]
			platformCode = tr[1]
		}
		return []any{
			server.feedRef,
			tripID,
			parseInt(row["stop_sequence"]),
			stopID,
			nullString(platformCode),
			nullString(row["stop_headsign"]),
			nullString(row["arrival_time"]),
			nullString(row["departure_time"]),
			parseNullableInt(row["pickup_type"]),
			parseNullableInt(row["drop_off_type"]),
			parseNullableInt(row["timepoint"]),
			parseNullableFloat(row["shape_dist_traveled"]),
			parseNullableInt(row["fare_units_traveled"]),
		}
	},
}

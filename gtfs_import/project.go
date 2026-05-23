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

type ProjectorTask struct {
	tableName string
	query     string
	extraArgs []any
	deps      []string
}

func (t ProjectorTask) NeedsRun(server *Server) (bool, error) {
	if t.tableName == "" {
		return true, nil
	}
	query := "SELECT EXISTS(SELECT 1 FROM " + t.tableName + " WHERE feed_ref = $1)"

	var exists bool
	err := server.db.QueryRow(query, server.feedRef).Scan(&exists)
	if err != nil {
		return true, err
	}
	return !exists, nil
}

func (t ProjectorTask) Group() string {
	return t.tableName
}

func (t ProjectorTask) Execute(server *Server, progress func(float64)) error {
	_, err := server.db.Exec(t.query, append([]any{server.feedRef}, t.extraArgs...)...)
	return err
}

func (t ProjectorTask) Cleanup(*Server) error { return nil }

func (t ProjectorTask) Dependencies() []string {
	return append([]string{"feed_ref"}, t.deps...)
}

var clearTripBounds = ProjectorTask{
	tableName: "gtfs_trip_bounds",
	query: `
	DELETE FROM gtfs_trip_bounds
	WHERE feed_ref = $1;
	`,
}

var calculateTripBounds = ProjectorTask{
	tableName: "gtfs_trip_bounds",
	deps:      []string{"clear_trip_bounds", "import_stop_times"},
	query: `
WITH bounds AS (
	SELECT
		feed_ref,
		trip_id,
		min(stop_sequence) AS start_sequence,
		max(stop_sequence) AS end_sequence
	FROM gtfs_stop_times
	WHERE feed_ref = $1
	GROUP BY feed_ref, trip_id
)
INSERT INTO gtfs_trip_bounds (
	feed_ref,
	trip_id,
	start_time,
	end_time,
	start_sequence,
	end_sequence,
	start_stop,
	end_stop
)
SELECT
	b.feed_ref,
	b.trip_id,
	COALESCE(start_st.departure_time, start_st.arrival_time) AS start_time,
	COALESCE(end_st.arrival_time, end_st.departure_time) AS end_time,
	b.start_sequence,
	b.end_sequence,
	start_st.stop_id AS start_stop,
	end_st.stop_id AS end_stop
FROM bounds b
JOIN gtfs_stop_times start_st
	ON start_st.feed_ref = b.feed_ref
   AND start_st.trip_id = b.trip_id
   AND start_st.stop_sequence = b.start_sequence
JOIN gtfs_stop_times end_st
	ON end_st.feed_ref = b.feed_ref
   AND end_st.trip_id = b.trip_id
   AND end_st.stop_sequence = b.end_sequence;`,
}

var clearStopEvents = ProjectorTask{
	tableName: "gtfs_stop_events",
	query: `
	DELETE FROM gtfs_stop_events
	WHERE feed_ref = $1;
	`,
}

var calculateStopEvents = ProjectorTask{
	tableName: "gtfs_stop_events",
	deps: []string{
		"clear_stop_events",
		"import_stop_times",
		"import_trips",
		"import_routes",
		"import_agencies",
		"calc_trip_bounds",
	},
	query: `
INSERT INTO gtfs_stop_events (
	feed_ref,
	mode,
	scheduled_time,
	service_id,
	service_date,

	trip_id,
	route_id,
	stop_sequence,
	stop_id,
	stop_headsign,
	event_type,
	timepoint,
	shape_dist_traveled,
	fare_units_traveled,

	terminal,
	first_stop,
	last_stop
)
SELECT
	st.feed_ref,
	ev.mode,
	((cd.date::timestamp + ev.stop_time) AT TIME ZONE a.agency_timezone),
	t.service_id,
	cd.date,

	t.trip_id,
	t.route_id,
	st.stop_sequence,
	st.stop_id,
	st.stop_headsign,
	COALESCE(ev.event_type, 0),
	st.timepoint,
	st.shape_dist_traveled,
	st.fare_units_traveled,

	st.stop_sequence = ev.terminal_sequence,
	st.stop_sequence = tb.start_sequence,
	st.stop_sequence = tb.end_sequence
FROM gtfs_stop_times st
JOIN gtfs_trips t
	ON t.feed_ref = st.feed_ref
   AND t.trip_id = st.trip_id
JOIN gtfs_routes r
	ON r.feed_ref = t.feed_ref
   AND r.route_id = t.route_id
JOIN gtfs_agency a
	ON a.feed_ref = r.feed_ref
   AND a.agency_id = r.agency_id
JOIN gtfs_calendar_dates cd
	ON cd.feed_ref = t.feed_ref
   AND cd.service_id = t.service_id
   AND cd.exception_type = 1
JOIN gtfs_trip_bounds tb
	ON tb.feed_ref = st.feed_ref
   AND tb.trip_id = st.trip_id
CROSS JOIN LATERAL (
	VALUES
		(
			'arrival'::gtfs_stop_event_mode,
			st.arrival_time,
			st.drop_off_type,
			tb.start_sequence
		),
		(
			'departure'::gtfs_stop_event_mode,
			st.departure_time,
			st.pickup_type,
			tb.end_sequence
		)
) AS ev(mode, stop_time, event_type, terminal_sequence)
WHERE st.feed_ref = $1
  AND ev.stop_time IS NOT NULL;
	`,
}

var calculateRTTSequence = ProjectorTask{
	tableName: "gtfs_trips",
	deps:      []string{"import_trips"},
	query: `
WITH seq AS (
	SELECT
		feed_ref,
		trip_id,
		row_number() OVER (
			PARTITION BY feed_ref, service_id, realtime_trip_id
			ORDER BY trip_id
		) AS n
	FROM gtfs_trips
	WHERE feed_ref = $1
	  AND realtime_trip_id IS NOT NULL
)
UPDATE gtfs_trips t
SET realtime_trip_sequence = seq.n
FROM seq
WHERE t.feed_ref = seq.feed_ref
  AND t.trip_id = seq.trip_id;
 `,
}

var calculateShapes = ProjectorTask{
	tableName: "gtfs_shapes",
	deps:      []string{"import_trips", "import_stops"},
	query: `
WITH missing AS (
	SELECT
		t.feed_ref,
		t.trip_id,
		'gtrip:' || t.trip_id AS shape_id
	FROM gtfs_trips t
	WHERE t.feed_ref = $1
	  AND NULLIF(t.shape_id, '') IS NULL
),
inserted_shapes AS (
	INSERT INTO gtfs_shapes (
		feed_ref,
		shape_id,
		shape_pt_sequence,
		shape_pt_lat,
		shape_pt_lon,
		shape_dist_traveled
	)
	SELECT
		m.feed_ref,
		m.shape_id,
		st.stop_sequence,
		s.stop_lat,
		s.stop_lon,
		NULL
	FROM missing m
	JOIN gtfs_stop_times st
		ON st.feed_ref = m.feed_ref
	   AND st.trip_id = m.trip_id
	JOIN gtfs_stops s
		ON s.feed_ref = st.feed_ref
	   AND s.stop_id = st.stop_id
	WHERE s.stop_lat IS NOT NULL
	  AND s.stop_lon IS NOT NULL
	ORDER BY
		m.feed_ref,
		m.trip_id,
		st.stop_sequence
	ON CONFLICT DO NOTHING
)
UPDATE gtfs_trips t
SET shape_id = m.shape_id
FROM missing m
WHERE t.feed_ref = m.feed_ref
  AND t.trip_id = m.trip_id
`,
}

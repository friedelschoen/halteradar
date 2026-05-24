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

SELECT DISTINCT ON (v.punctuality, v.vehicle_number)
	v.data_owner_code,
	v.vehicle_number,
	v.status,
	EXTRACT(EPOCH FROM v.event_timestamp)::bigint AS last_seen,
	v.user_stop_code,
	v.passage_sequence_number,
	v.realtime_trip_id,
	v.block_code,
	COALESCE(v.punctuality, 0) AS punctuality,
	v.rd_x,
	v.rd_y,
    v.lat,
    v.lon,

	s.stop_id,
	s.stop_name,
	s.platform_code,

	t.trip_id,
	t.trip_headsign,
	r.route_id,
	r.route_short_name,
	r.route_color,
	r.route_text_color
FROM kv6_current_vehicle v
JOIN active_gtfs_stops s
	ON s.stop_code = v.user_stop_code
LEFT JOIN active_gtfs_trips t
    ON t.realtime_trip_id = v.realtime_trip_id
   AND t.realtime_trip_sequence = 1
LEFT JOIN active_gtfs_routes r
    ON r.route_id = t.route_id
WHERE v.status IN ('INIT', 'ONSTOP', 'ARRIVAL', 'DEPARTURE')
    AND (s.stop_id = $1 OR s.parent_station = $1)
ORDER BY
    v.punctuality DESC,
	v.vehicle_number,
    v.event_timestamp DESC;

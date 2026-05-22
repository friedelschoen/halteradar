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

CREATE INDEX IF NOT EXISTS idx_stop_events_lookup
ON gtfs_stop_events (
	stop_id,
	mode,
	scheduled_time
);

CREATE INDEX IF NOT EXISTS idx_stops_parent_station
ON gtfs_stops (
	parent_station
);

CREATE INDEX IF NOT EXISTS idx_kv6_current_trip_lookup
ON kv6_current_trip (
	operating_day,
	realtime_trip_id
);

CREATE INDEX IF NOT EXISTS idx_kv6_trip_stop_status_lookup
ON kv6_trip_stop_status (
	operating_day,
	realtime_trip_id,
	user_stop_code
);

CREATE INDEX IF NOT EXISTS idx_gtfs_trips_route_service
ON gtfs_trips (
	route_id,
	service_id
);

CREATE INDEX IF NOT EXISTS idx_calendar_dates_active
ON gtfs_calendar_dates (
	service_id,
	date
)
WHERE exception_type = 1;

CREATE INDEX IF NOT EXISTS idx_stop_events_routes
ON gtfs_stop_events (
	stop_id,
	scheduled_time,
	route_id
);

CREATE INDEX IF NOT EXISTS idx_kv6_block_history_lookup
ON kv6_block_trip_history (
	operating_day,
	block_code,
	first_seen
);

CREATE INDEX IF NOT EXISTS idx_kv6_current_trip_block
ON kv6_current_trip (
	operating_day,
	block_code
);

CREATE INDEX IF NOT EXISTS idx_gtfs_trips_realtime
ON gtfs_trips (
	realtime_trip_id
);

CREATE INDEX IF NOT EXISTS idx_stop_events_realtime
ON gtfs_stop_events (
	realtime_trip_id
);

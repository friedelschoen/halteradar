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

WITH active_feed AS (
	SELECT id
	FROM gtfs_feeds
	WHERE active = true
	ORDER BY imported_at DESC
	LIMIT 1
)
SELECT
	stop_id,
	stop_code,
	stop_name,
	parent_station,
	platform_code
FROM gtfs_stops s
JOIN active_feed af ON af.id = s.feed_ref
WHERE
	lower(stop_name) LIKE '%' || lower($1) || '%'
	OR lower(stop_code) = lower($1)
	OR lower(stop_id) = lower($1)
ORDER BY
	stop_name
LIMIT 20;

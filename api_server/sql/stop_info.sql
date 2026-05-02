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

SELECT 
    stop_id,
	stop_code,
	stop_name,
	stop_lat,
	stop_lon,
	location_type,
	parent_station,
	stop_timezone,
	wheelchair_boarding,
	platform_code,
	zone_id
FROM gtfs_stops
WHERE stop_id = $1

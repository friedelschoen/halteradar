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

type CollectTask struct {
	deps    []string
	execute func(server *Server, progress func(float64)) error
}

func (t CollectTask) NeedsRun(server *Server) (bool, error) {
	return true, nil
}

func (t CollectTask) Execute(server *Server, progress func(float64)) error {
	return t.execute(server, progress)
}

func (t CollectTask) Cleanup(*Server) error { return nil }

func (t CollectTask) Group() string { return "" }

func (t CollectTask) Dependencies() []string {
	return append([]string{"feed_ref", "archive"}, t.deps...)
}

var collectRoutesTask = CollectTask{
	execute: func(server *Server, progress func(float64)) error {
		server.routes = make(map[string]struct{})
		return server.iterCSV(progress, "routes.txt", func(row map[string]string) error {
			agencyID := row["agency_id"]
			if _, ok := server.agencies[agencyID]; !ok {
				return nil
			}
			routeID := row["route_id"]
			server.routes[routeID] = struct{}{}
			return nil
		})
	},
}

var collectStopsTask = CollectTask{
	deps: []string{"collect_trips", "collect_stop_times"},
	execute: func(server *Server, progress func(float64)) error {
		stations := make(map[string]struct{})
		err := server.iterCSV(progress, "stops.txt", func(row map[string]string) error {
			stopID := row["stop_id"]
			if _, ok := server.stops[stopID]; !ok {
				return nil
			}
			parentStation := row["parent_station"]
			if parentStation != "" {
				stations[parentStation] = struct{}{}
			}
			return nil
		})
		for stop := range stations {
			server.stops[stop] = struct{}{}
		}
		server.stopsCollected = true
		return err
	},
}

var collectTripsTask = CollectTask{
	deps: []string{"collect_routes"},
	execute: func(server *Server, progress func(float64)) error {
		server.trips = make(map[string]struct{})
		server.services = make(map[string]struct{})
		server.shapes = make(map[string]struct{})
		return server.iterCSV(progress, "trips.txt", func(row map[string]string) error {
			routeID := row["route_id"]
			if _, ok := server.routes[routeID]; !ok {
				return nil
			}
			tripID := row["trip_id"]
			server.trips[tripID] = struct{}{}
			serviceID := row["service_id"]
			server.services[serviceID] = struct{}{}
			shapeID := row["shape_id"]
			if shapeID != "" {
				server.shapes[shapeID] = struct{}{}
			}
			return nil
		})
	},
}

var collectStopTimesTask = CollectTask{
	deps: []string{"collect_trips"},
	execute: func(server *Server, progress func(float64)) error {
		server.stops = make(map[string]struct{})
		return server.iterCSV(progress, "stop_times.txt", func(row map[string]string) error {
			tripID := row["trip_id"]
			if _, ok := server.trips[tripID]; !ok {
				return nil
			}
			stopID := row["stop_id"]
			server.stops[stopID] = struct{}{}
			return nil
		})
	},
}

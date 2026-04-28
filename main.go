package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dylandreimerink/go-rijksdriehoek"
	zmq "github.com/pebbe/zmq4"
)

//go:generate protoc --go_out=. --go_opt=module=github.com/friedelschoen/depatures -Iproto proto/gtfs-realtime-OVapi.proto proto/gtfs-realtime.proto

const (
	staticURL = "https://gtfs.ovapi.nl/nl/gtfs-nl.zip"

	defaultStopID = "stoparea:449933" /* change this */
)

type Static struct {
	Stops       map[string]Stop
	Parents     map[string][]string
	Routes      map[string]Route
	Trips       map[string]Trip
	StopTimes   map[string][]StopTime
	Calendar    map[string]Calendar
	DateChanges map[string]map[string]int
	TripMaxSeq  map[string]int
}

type Stop struct {
	ID       string
	Name     string
	Platform string
}

type Route struct {
	ID        string
	ShortName string
	LongName  string
}

type Trip struct {
	ID        string
	RouteID   string
	ServiceID string
	Headsign  string
	Realtime  string
	BlockCode string
}

type StopTime struct {
	TripID       string
	StopID       string
	Departure    int
	Headsign     string
	StopSequence int
}

type Calendar struct {
	Weekdays [7]bool
	Start    time.Time
	End      time.Time
}

type Realtime struct {
	Trips      map[string]RealtimeTrip
	Vehicles   map[string]RealtimeVehicle
	BlockCodes map[string]string
	Updated    time.Time
}

type RealtimeTrip struct {
	DelaySeconds int
	Cancelled    bool
	Updated      time.Time
}

type RealtimeVehicle struct {
	ID      string
	Lat     float64
	Lon     float64
	Updated time.Time
}

type Server struct {
	static Static

	mu sync.RWMutex
	rt Realtime
}

type StopInfo struct {
	Name       string      `json:"name"`
	Departures []Departure `json:"departures"`
}

type Departure struct {
	Line          string   `json:"line"`
	RouteID       string   `json:"route_id"`
	TripID        string   `json:"trip_id"`
	Headsign      string   `json:"headsign"`
	Platform      string   `json:"platform"`
	Date          string   `json:"date"`
	ScheduledTime string   `json:"scheduled_time"`
	RealtimeTime  string   `json:"realtime_time"`
	DelayMinutes  int      `json:"delay_minutes"`
	Cancelled     bool     `json:"cancelled"`
	Vehicle       *Vehicle `json:"vehicle,omitempty"`
	BlockCode     string   `json:"blockcode"`
	Terminal      bool     `json:"terminal"`
}

type Vehicle struct {
	ID  string  `json:"id"`
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func main() {
	cache := cacheFile("cache/static.zip")
	st, err := loadStatic(staticURL, &cache)
	if err != nil {
		log.Fatal(err)
	}
	cache.buffer = nil

	s := &Server{static: st}
	s.refreshLoop()

	http.HandleFunc("/", s.index)
	http.HandleFunc("/api/departures", s.departures)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

const ndovEndpoint = "tcp://pubsub.besteffort.ndovloket.nl:7658"

var ndovTopics = []string{
	"/QBUZZ/KV15messages",
	"/QBUZZ/KV17cvlinfo",
	"/QBUZZ/KV6posinfo",
}

func (s *Server) refreshLoopOnline() {
	ctx, err := zmq.NewContext()
	if err != nil {
		log.Println("zmq context:", err)
		return
	}
	defer ctx.Term()

	sub, err := ctx.NewSocket(zmq.SUB)
	if err != nil {
		log.Println("zmq socket:", err)
		return
	}
	defer sub.Close()

	for _, topic := range ndovTopics {
		if err := sub.SetSubscribe(topic); err != nil {
			log.Println("zmq subscribe:", topic, err)
			return
		}
	}

	if err := sub.Connect(ndovEndpoint); err != nil {
		log.Println("zmq connect:", err)
		return
	}

	log.Println("connected to NDOV", ndovEndpoint)

	s.mu.Lock()
	s.rt = Realtime{
		Trips:      map[string]RealtimeTrip{},
		BlockCodes: map[string]string{},
		Vehicles:   map[string]RealtimeVehicle{},
		Updated:    time.Now(),
	}
	s.mu.Unlock()

	for {
		parts, err := sub.RecvMessageBytes(0)
		if err != nil {
			log.Println("zmq recv:", err)
			time.Sleep(time.Second)
			continue
		}
		if len(parts) < 2 {
			continue
		}

		topic := string(parts[0])

		for _, part := range parts[1:] {
			xmlbuf, err := ungzip(part)
			if err != nil {
				log.Println("gzip:", topic, err)
				continue
			}

			switch topic {
			case "/QBUZZ/KV6posinfo":
				s.handleKV6(xmlbuf)

			case "/QBUZZ/KV17cvlinfo":
				s.handleKV17(xmlbuf)

			case "/QBUZZ/KV15messages":
				/* Reisberichten. Voor je tabel voorlopig niet nodig. */
			}
		}
	}
}

func (s *Server) refreshLoop() {
	log.Println("connected to NDOV", ndovEndpoint)

	s.mu.Lock()
	s.rt = Realtime{
		Trips:    map[string]RealtimeTrip{},
		Vehicles: map[string]RealtimeVehicle{},
		Updated:  time.Now(),
	}
	s.mu.Unlock()

	f, err := os.Open("test/log.txt")
	if err != nil {
		log.Fatalln(err)
		return
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	topic := ""
	var buffer bytes.Buffer
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if strings.HasPrefix(line, "---") && strings.HasSuffix(line, "---") {
			if topic != "" {
				switch topic {
				case "/QBUZZ/KV6posinfo":
					s.handleKV6(buffer.Bytes())

				case "/QBUZZ/KV17cvlinfo":
					s.handleKV17(buffer.Bytes())
				}
			}

			topic = strings.TrimSpace(line[3 : len(line)-3])
			buffer.Reset()
		} else {
			buffer.Write(scan.Bytes())
		}
	}
	if topic != "" {
		switch topic {
		case "/QBUZZ/KV6posinfo":
			s.handleKV6(buffer.Bytes())

		case "/QBUZZ/KV17cvlinfo":
			s.handleKV17(buffer.Bytes())
		}
	}

	fmt.Println("ndov loaded")
}

type kv6Subscription struct {
	XMLName xml.Name    `xml:"VV_TM_PUSH"`
	PosInfo kv6Envelope `xml:"KV6posinfo"`
}

type kv6Envelope struct {
	XMLName xml.Name  `xml:"KV6posinfo"`
	Items   []kv6Item `xml:",any"`
}

type kv6Item struct {
	XMLName xml.Name

	DataOwnerCode      string `xml:"dataownercode"`
	LinePlanningNumber string `xml:"lineplanningnumber"`
	OperatingDay       string `xml:"operatingday"`
	JourneyNumber      string `xml:"journeynumber"`
	UserStopCode       string `xml:"userstopcode"`

	VehicleNumber string  `xml:"vehiclenumber"`
	BlockCode     string  `xml:"blockcode"`
	Punctuality   int     `xml:"punctuality"`
	RDX           float64 `xml:"rd-x"`
	RDY           float64 `xml:"rd-y"`
}

func (s *Server) handleKV6(b []byte) {
	var env kv6Subscription
	if err := xml.Unmarshal(b, &env); err != nil {
		log.Println("kv6 xml:", err)
		return
	}

	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rt.Trips == nil {
		s.rt.Trips = map[string]RealtimeTrip{}
	}
	if s.rt.Vehicles == nil {
		s.rt.Vehicles = map[string]RealtimeVehicle{}
	}
	if s.rt.BlockCodes == nil {
		s.rt.BlockCodes = map[string]string{}
	}

	for _, item := range env.PosInfo.Items {
		key := ndovJourneyKey(item.DataOwnerCode, item.LinePlanningNumber, item.JourneyNumber)
		if key == "" {
			continue
		}

		switch strings.ToUpper(item.XMLName.Local) {
		case "DELAY", "ARRIVAL", "DEPARTURE", "ONSTOP", "ONROUTE":
			rt := s.rt.Trips[key]
			rt.DelaySeconds = item.Punctuality
			rt.Updated = now

			s.rt.Trips[key] = rt
		}

		if item.VehicleNumber != "" {
			v := RealtimeVehicle{
				ID:      item.VehicleNumber,
				Updated: now,
			}
			v.Lat, v.Lon = rijksdriehoek.RDtoWGS84(item.RDX, item.RDY)
			s.rt.Vehicles[key] = v
		}

		if item.BlockCode != "" {
			s.rt.BlockCodes[key] = item.BlockCode
		}
	}

	s.rt.Updated = now
}

type kv17Envelope struct {
	XMLName xml.Name
	Items   []kv17Item `xml:",any"`
}

type kv17Item struct {
	XMLName xml.Name

	DataOwnerCode      string `xml:"dataownercode,attr"`
	LinePlanningNumber string `xml:"lineplanningnumber,attr"`
	OperatingDay       string `xml:"operatingday,attr"`
	JourneyNumber      string `xml:"journeynumber,attr"`
}

func (s *Server) handleKV17(b []byte) {
	var env kv17Envelope
	if err := xml.Unmarshal(b, &env); err != nil {
		log.Println("kv17 xml:", err)
		return
	}

	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rt.Trips == nil {
		s.rt.Trips = map[string]RealtimeTrip{}
	}

	for _, item := range env.Items {
		key := ndovJourneyKey(item.DataOwnerCode, item.LinePlanningNumber, item.JourneyNumber)
		if key == "" {
			continue
		}

		name := strings.ToUpper(item.XMLName.Local)

		rt := s.rt.Trips[key]
		rt.Updated = now

		switch name {
		case "CANCELJOURNEY", "CANCELLEDJOURNEY", "DELETEJOURNEY":
			rt.Cancelled = true
		}

		s.rt.Trips[key] = rt
	}

	s.rt.Updated = now
}

func ndovJourneyKey(owner, line, journey string) string {
	if owner == "" || line == "" || journey == "" {
		return ""
	}
	return owner + ":" + line + ":" + journey
}

func ungzip(b []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

type cache struct {
	path   string
	buffer []byte
	time   time.Time
}

func cacheFile(path string) (c cache) {
	c.path = path

	stat, err := os.Stat(path)
	if err != nil {
		log.Printf("unable to stat cache %s: %v\n", path, err)
		return c
	}

	b, err := os.ReadFile(path)
	if err != nil {
		log.Printf("unable to read cache %s: %v\n", path, err)
		return c
	}

	c.time = stat.ModTime()
	c.buffer = b
	return
}

func (cache *cache) update(url string) error {
	if time.Since(cache.time) < 24*time.Hour {
		return nil
	}
	fmt.Printf("updating %s\n", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "DepatureBot/0.1 <derfriedmundschoen@gmail.com>")
	if !cache.time.IsZero() {
		req.Header.Set("If-Modified-Since", cache.time.Format(time.RFC1123))
	}
	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	log.Printf("%s: %s\n", url, resp.Status)
	switch resp.StatusCode {
	case http.StatusOK:
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		cache.buffer = b
		cache.time = time.Now()

		if err := os.WriteFile(cache.path, cache.buffer, 0644); err != nil {
			log.Printf("unable to write to cache %s: %v\n", cache.path, err)
		}

	case http.StatusNotModified:
		/* it's fine, we'll use cache! */

	default:
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	return nil
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	stop := r.URL.Query().Get("stop")
	if stop == "" {
		stop = defaultStopID
	}

	page.Execute(w, map[string]string{
		"StopID": stop,
	})
}

func lastNumber(s string) string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r < '0' || r > '9'
	})

	if len(fields) == 0 {
		return ""
	}

	return fields[len(fields)-1]
}

func (s *Server) isLastStop(st StopTime) bool {
	return st.StopSequence == s.static.TripMaxSeq[st.TripID]
}

func (s *Server) departures(w http.ResponseWriter, r *http.Request) {
	stopID := r.URL.Query().Get("stop")
	if stopID == "" {
		stopID = defaultStopID
	}

	now := time.Now()
	base := midnight(now)

	s.mu.RLock()
	rt := s.rt
	s.mu.RUnlock()

	stops := []string{stopID}
	if chld, ok := s.static.Parents[stopID]; ok {
		stops = append(stops, chld...)
	}

	var out StopInfo
	out.Name = s.static.Stops[stopID].Name

	for _, stop := range stops {
		for _, st := range s.static.StopTimes[stop] {
			trip, ok := s.static.Trips[st.TripID]
			if !ok || !s.serviceActive(trip.ServiceID, now) {
				continue
			}

			sched := base.Add(time.Duration(st.Departure) * time.Second)
			if sched.Before(now.Add(-10*time.Minute)) || sched.After(now.Add(3*time.Hour)) {
				continue
			}

			route := s.static.Routes[trip.RouteID]
			headsign := st.Headsign
			if headsign == "" {
				headsign = trip.Headsign
			}

			dep := Departure{
				Line:          route.ShortName,
				RouteID:       route.ID,
				TripID:        trip.ID,
				Headsign:      headsign,
				Platform:      s.static.Stops[stop].Platform,
				Date:          sched.Format(time.DateOnly),
				ScheduledTime: sched.Format("15:04"),
				RealtimeTime:  sched.Format("15:04"),
				Terminal:      s.isLastStop(st),
				BlockCode:     trip.BlockCode,
			}

			if bc, ok := rt.BlockCodes[trip.Realtime]; ok {
				dep.BlockCode = bc
			}

			if rtu, ok := rt.Trips[trip.Realtime]; ok {
				realtime := sched.Add(time.Duration(rtu.DelaySeconds) * time.Second)

				dep.RealtimeTime = realtime.Format("15:04")
				dep.DelayMinutes = rtu.DelaySeconds / 60
				dep.Cancelled = rtu.Cancelled
			}

			if v, ok := rt.Vehicles[trip.Realtime]; ok && sched.After(now) {
				dep.Vehicle = &Vehicle{
					ID:  v.ID,
					Lat: v.Lat,
					Lon: v.Lon,
				}
			}
			out.Departures = append(out.Departures, dep)
		}
	}

	sort.Slice(out.Departures, func(i, j int) bool {
		return out.Departures[i].RealtimeTime < out.Departures[j].RealtimeTime
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func loadStatic(url string, cache *cache) (Static, error) {
	//	if err := cache.update(url); err != nil {
	//		return Static{}, err
	//	}
	fmt.Println("cache: ", len(cache.buffer))
	zr, err := zip.NewReader(bytes.NewReader(cache.buffer), int64(len(cache.buffer)))
	if err != nil {
		return Static{}, err
	}

	st := Static{
		Stops:       map[string]Stop{},
		Parents:     map[string][]string{},
		Routes:      map[string]Route{},
		Trips:       map[string]Trip{},
		TripMaxSeq:  map[string]int{},
		StopTimes:   map[string][]StopTime{},
		Calendar:    map[string]Calendar{},
		DateChanges: map[string]map[string]int{},
	}

	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}

	readCSV(files["stops.txt"], func(row map[string]string) {
		st.Stops[row["stop_id"]] = Stop{
			ID:       row["stop_id"],
			Name:     row["stop_name"],
			Platform: row["platform_code"],
		}
		if p := row["parent_station"]; p != "" {
			st.Parents[p] = append(st.Parents[p], row["stop_id"])
		}
	})

	readCSV(files["routes.txt"], func(row map[string]string) {
		st.Routes[row["route_id"]] = Route{
			ID:        row["route_id"],
			ShortName: firstNonEmpty(row["route_short_name"], row["route_long_name"]),
			LongName:  row["route_long_name"],
		}
	})

	readCSV(files["trips.txt"], func(row map[string]string) {
		st.Trips[row["trip_id"]] = Trip{
			ID:        row["trip_id"],
			RouteID:   row["route_id"],
			ServiceID: row["service_id"],
			Headsign:  row["trip_headsign"],
			Realtime:  row["realtime_trip_id"],
		}
	})

	readCSV(files["stop_times.txt"], func(row map[string]string) {
		dep, err := parseGTFSTime(firstNonEmpty(row["departure_time"], row["arrival_time"]))
		if err != nil {
			return
		}

		stopID := row["stop_id"]
		seq, _ := strconv.Atoi(row["stop_sequence"])
		st.StopTimes[stopID] = append(st.StopTimes[stopID], StopTime{
			TripID:       row["trip_id"],
			StopID:       stopID,
			Departure:    dep,
			Headsign:     row["stop_headsign"],
			StopSequence: seq,
		})

		if seq > st.TripMaxSeq[row["trip_id"]] {
			st.TripMaxSeq[row["trip_id"]] = seq
		}
	})

	readCSV(files["calendar.txt"], func(row map[string]string) {
		var wd [7]bool
		wd[time.Monday] = row["monday"] == "1"
		wd[time.Tuesday] = row["tuesday"] == "1"
		wd[time.Wednesday] = row["wednesday"] == "1"
		wd[time.Thursday] = row["thursday"] == "1"
		wd[time.Friday] = row["friday"] == "1"
		wd[time.Saturday] = row["saturday"] == "1"
		wd[time.Sunday] = row["sunday"] == "1"

		st.Calendar[row["service_id"]] = Calendar{
			Weekdays: wd,
			Start:    parseDate(row["start_date"]),
			End:      parseDate(row["end_date"]),
		}
	})

	readCSV(files["calendar_dates.txt"], func(row map[string]string) {
		sid := row["service_id"]
		date := row["date"]
		ex, _ := strconv.Atoi(row["exception_type"])

		if st.DateChanges[sid] == nil {
			st.DateChanges[sid] = map[string]int{}
		}
		st.DateChanges[sid][date] = ex
	})

	for stopID := range st.StopTimes {
		sort.Slice(st.StopTimes[stopID], func(i, j int) bool {
			return st.StopTimes[stopID][i].Departure < st.StopTimes[stopID][j].Departure
		})
	}

	return st, nil
}

func readCSV(f *zip.File, fn func(map[string]string)) {
	if f == nil {
		return
	}

	rc, err := f.Open()
	if err != nil {
		return
	}
	defer rc.Close()

	r := csv.NewReader(rc)
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return
	}

	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		row := map[string]string{}
		for i, h := range header {
			if i < len(rec) {
				row[h] = rec[i]
			}
		}
		fn(row)
	}
}

func (s *Server) serviceActive(serviceID string, t time.Time) bool {
	key := t.Format("20060102")

	if change := s.static.DateChanges[serviceID][key]; change == 1 {
		return true
	} else if change == 2 {
		return false
	}

	cal, ok := s.static.Calendar[serviceID]
	if !ok {
		return false
	}

	d := midnight(t)
	if d.Before(cal.Start) || d.After(cal.End) {
		return false
	}

	return cal.Weekdays[t.Weekday()]
}

func parseGTFSTime(v string) (int, error) {
	p := strings.Split(v, ":")
	if len(p) != 3 {
		return 0, fmt.Errorf("bad GTFS time: %q", v)
	}

	h, _ := strconv.Atoi(p[0])
	m, _ := strconv.Atoi(p[1])
	s, _ := strconv.Atoi(p[2])

	return h*3600 + m*60 + s, nil
}

func parseDate(v string) time.Time {
	t, _ := time.ParseInLocation("20060102", v, time.Local)
	return t
}

func midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

//go:embed departures.html
var pageContents string

var page = template.Must(template.New("index").Parse(pageContents))

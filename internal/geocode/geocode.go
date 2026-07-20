// Package geocode does offline reverse geocoding: GPS coordinates -> nearest
// city and country name, using an embedded GeoNames cities15000 dataset.
//
// Data: GeoNames (https://www.geonames.org/), licensed CC BY 4.0.
package geocode

import (
	"bufio"
	"compress/gzip"
	"embed"
	"math"
	"strconv"
	"strings"
	"sync"
)

//go:embed cities.tsv.gz
var data embed.FS

type city struct {
	name    string
	country string // ISO alpha-2
	lat     float64
	lon     float64
}

var (
	cities []city
	once   sync.Once
)

func load() {
	f, err := data.Open("cities.tsv.gz")
	if err != nil {
		return
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return
	}
	defer gz.Close()
	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		parts := strings.Split(sc.Text(), "\t")
		if len(parts) < 4 {
			continue
		}
		lat, err1 := strconv.ParseFloat(parts[2], 64)
		lon, err2 := strconv.ParseFloat(parts[3], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		cities = append(cities, city{name: parts[0], country: parts[1], lat: lat, lon: lon})
	}
}

// Lookup returns "City, Country" for the given coordinates, or "" if no data is
// available. Uses nearest-neighbor over the embedded city list.
func Lookup(lat, lon float64) string {
	once.Do(load)
	if len(cities) == 0 {
		return ""
	}
	cosLat := math.Cos(lat * math.Pi / 180)
	best := -1
	bestD := math.MaxFloat64
	for i := range cities {
		dLat := cities[i].lat - lat
		dLon := (cities[i].lon - lon) * cosLat
		d := dLat*dLat + dLon*dLon // squared equirectangular distance; fine for nearest
		if d < bestD {
			bestD = d
			best = i
		}
	}
	if best < 0 {
		return ""
	}
	c := cities[best]
	country := countryNames[c.country]
	if country == "" {
		country = c.country
	}
	if country == "" {
		return c.name
	}
	return c.name + ", " + country
}

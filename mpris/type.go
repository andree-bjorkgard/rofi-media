package mpris

import (
	"fmt"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

type Media struct {
	ID     string
	Length time.Duration
	ArtURL string

	Artist      string
	Album       string
	AlbumArtist string
	Title       string
	Genre       string
	Year        int8

	URL string
}

func decodeMetadata(metadata any, m *Media) error {
	metadataMap, ok := metadata.(map[string]dbus.Variant)
	if !ok {
		return fmt.Errorf("mpris.decodeMetadata: metadata is not a valid structure")
	}

	for key, val := range metadataMap {
		switch key {
		case "mpris:trackid":
			if v, ok := val.Value().(string); ok {
				m.ID = v
			}
		case "mpris:length":
			if v, ok := val.Value().(int64); ok {
				m.Length = time.Duration(v * int64(time.Microsecond))
			}

		case "mpris:artUrl":
			if v, ok := val.Value().(string); ok {
				m.ArtURL = v
			}

		case "xesam:album":
			if v, ok := val.Value().(string); ok {
				m.Album = v
			}

		case "xesam:albumArtist":
			if v, ok := val.Value().(string); ok {
				m.AlbumArtist = v
			}

		case "xesam:artist":
			if v, ok := val.Value().(string); ok {
				m.Artist = v
				continue
			}
			if v, ok := val.Value().([]string); ok {
				m.Artist = strings.Join(v, ", ")
				continue
			}

		case "xesam:asText":
		case "xesam:audioBPM":
		case "xesam:autoRating":
		case "xesam:comment":
		case "xesam:composer":
		case "xesam:contentCreated":
			if v, ok := val.Value().(string); ok {
				if ti, err := time.Parse(time.RFC3339, v); err == nil {
					m.Year = int8(ti.Year())
				}
			}
		case "xesam:discNumber":
		case "xesam:firstUsed":
		case "xesam:genre":
			if v, ok := val.Value().([]string); ok {
				m.Genre = strings.Join(v, ", ")
			}
		case "xesam:lastUsed":
		case "xesam:lyricist":
		case "xesam:title":
			if v, ok := val.Value().(string); ok {
				m.Title = v
			}

		case "xesam:trackNumber":
		case "xesam:url":
			if v, ok := val.Value().(string); ok {
				m.URL = v
			}
		case "xesam:useCount":
		case "xesam:userRating":
		}

	}

	return nil
}

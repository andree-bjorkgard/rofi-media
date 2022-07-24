package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/ingentingalls/rofi"
	"github.com/ingentingalls/rofi-media/mpris"
)

const dbusDest = "org.freedesktop.DBus"
const dbusObjectPath = "/org/freedesktop/DBus"

func main() {
	var opts rofi.Options

	conn, err := dbus.SessionBus()
	if err != nil {
		log.Fatalf("dbusnotify: could not create a connection to the bus: %s", err)
	}

	obj := conn.Object(dbusDest, dbusObjectPath)
	resp := obj.Call("org.freedesktop.DBus.ListNames", dbus.Flags(0))
	if resp.Err != nil {
		log.Fatalf("listnames: %s", resp.Err)
	}

	var names []string
	if err := resp.Store(&names); err != nil {
		log.Fatalf("could not get names: %s", err)
	}

	var players []mpris.Player
	for _, name := range names {
		if mpris.HasValidDestinationName(name) {
			player, err := mpris.NewPlayer(conn, name)
			if err != nil {
				log.Printf("Could not create a new player from %s: %s", name, err)
			}
			players = append(players, player)
		}
	}

	if v := rofi.GetValue(); v != nil {
		selected, others := separatePlayers(players, v.Value)
		switch v.Cmd {
		case "pause", "pause-open":
			if err := selected.Pause(); err != nil {
				log.Printf("Could not pause (%s): %s", selected.Name, err)
			}
			if v.Cmd == "pause-open" {
				time.Sleep(66 * time.Millisecond)
				opts = showControls(selected, *v)
			}

		case "play", "play-open":
			if err := selected.Play(); err != nil {
				log.Printf("Could not play (%s): %s", selected.Name, err)
			}

			for _, p := range others {
				if err := p.Pause(); err != nil {
					log.Printf("Could not play (%s): %s", p.Name, err)
				}
			}

			if v.Cmd == "play-open" {
				time.Sleep(66 * time.Millisecond)
				opts = showControls(selected, *v)
				rofi.SetActive("0")
			}

		case "previous-open":
			opts = showControls(selected, *v)
			fallthrough
		case "previous":
			if err := selected.Previous(); err != nil {
				log.Printf("Could not play previous track (%s): %s", selected.Name, err)
			}

		case "next-open":
			showControls(selected, *v)
			opts = showControls(selected, *v)
			fallthrough
		case "next":
			if err := selected.Next(); err != nil {
				log.Printf("Could not play next track (%s): %s", selected.Name, err)
			}

		case "controls":
			opts = showControls(selected, *v)

		default:
			return
		}
	} else {
		opts = showAllPlayers(players)
	}

	rofi.SetPrompt("Players")
	rofi.EnableHotkeys()
	rofi.EnableMarkup()
	for _, opt := range opts {
		opt.Print()
	}
}

func separatePlayers(players []mpris.Player, name string) (mpris.Player, []mpris.Player) {
	var selected mpris.Player
	var others []mpris.Player
	for _, p := range players {
		if name == p.Name {
			selected = p
			continue
		}

		others = append(others, p)
	}

	return selected, others
}

func showControls(player mpris.Player, v rofi.Value) []rofi.Option {
	var opts []rofi.Option
	if player.IsPlaying() {
		opts = append(opts, rofi.Option{
			Name:  "Pause",
			Cmds:  []string{"pause-open"},
			Icon:  "player_pause",
			Value: v.Value,
		})
	} else {
		opts = append(opts, rofi.Option{
			Name:  "Play",
			Cmds:  []string{"play-open"},
			Icon:  "player_play",
			Value: v.Value,
		})
	}

	opts = append(opts,
		rofi.Option{
			Name:  "Previous",
			Cmds:  []string{"previous-open"},
			Icon:  "player_rew",
			Value: v.Value,
		},
		rofi.Option{
			Name:  "Next",
			Cmds:  []string{"next-open"},
			Icon:  "player_fwd",
			Value: v.Value,
		},
	)

	return opts
}

func showAllPlayers(players []mpris.Player) []rofi.Option {
	var opts []rofi.Option
	for _, player := range players {
		m, _ := player.GetMetadata()

		title := formatTitle(m, player.Short, player.IsPlaying())
		category := fmt.Sprintf("<span color=\"#C3C3C3\">%s</span>", player.Short)
		icon := getIcon(player.Name, m.ID, m.ArtURL)

		if player.Name == title {
			category = ""
		}

		baseCmd := "pause"
		if !player.IsPlaying() {
			baseCmd = "play"
		}

		opts = append(opts, rofi.Option{
			Name:        title,
			Category:    category,
			Icon:        icon,
			IsMultiline: true,
			Cmds:        []string{baseCmd, "controls"},
			Value:       player.Name,
		})
	}

	return opts
}

var localImageDir = path.Join(os.TempDir(), "/rofi-media")

func getIconFromURL(name, url string) string {
	// Needs any extension to work. doesnt matter which
	p := path.Join(localImageDir, strings.ReplaceAll(strings.TrimPrefix(name, "/"), "/", "-")+".img")

	if fs.ValidPath(p) {
		return p
	}

	ctx := context.Background()

	ctx, cancel := context.WithTimeout(ctx, time.Duration(time.Second))
	defer cancel()

	var reader io.ReadCloser

	http.NewRequestWithContext(ctx, http.MethodGet, url, reader)
	resp, err := http.Get(url)
	if err != nil {
		return ""

	}
	defer resp.Body.Close()

	// Do we need other 2xx codes here?
	if resp.StatusCode != http.StatusOK && !strings.Contains(resp.Header.Get("Content-Type"), "image/") {
		log.Printf("Not a valid icon: %s\n", err)
		return ""
	}

	if err := os.MkdirAll(localImageDir, os.ModePerm); err != nil {
		log.Printf("Error while creating path: %s\n", err)
		return ""
	}

	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		log.Printf("Error while opening icon file: %s\n", err)
		return ""
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		log.Printf("Error while writing icon into file: %s\n", err)
	}

	return p
}

func formatTitle(m mpris.Track, shortname string, isPlaying bool) string {
	title := ""
	if isPlaying {
		title = " "
	}
	if m.Title != "" {
		title += m.Title
		if m.Artist != "" {
			title = fmt.Sprintf("%s\r%s", title, m.Artist)
		}
		return title
	}

	if m.URL != "" {
		return title + path.Base(m.URL)
	}

	return title + shortname
}

func getIcon(name, id, url string) string {
	if url != "" {
		icon := getIconFromURL(id, url)
		if icon != "" {
			return icon
		}
	}

	switch {
	case strings.Contains(name, "chromium"):
		return "google-chrome"
	case strings.Contains(name, "spotify"):
		return "spotify"
	case strings.Contains(name, "vlc"):
		return "vlc"
	default:
		return ""
	}
}

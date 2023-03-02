package main

import (
	"context"
	"fmt"
	"html"
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
const dbusInterface = "org.freedesktop.DBus"
const dbusObjectPath = "/org/freedesktop/DBus"
const memberNameOwnerChanged = "NameOwnerChanged"
const signalNameOwnerChanged = dbusInterface + "." + memberNameOwnerChanged

func main() {
	var players []mpris.Player
	var currentView rofi.Value

	model, eventCh := rofi.NewRofiBlock()
	model.Prompt = "Players"
	model.Message = "Loading players..."
	model.Render()

	conn, err := dbus.SessionBus()
	if err != nil {
		log.Fatalf("dbusnotify: could not create a connection to the bus: %s", err)
	}

	conn.AddMatchSignal(
		dbus.WithMatchObjectPath(dbusObjectPath),
		dbus.WithMatchInterface(dbusInterface),
		dbus.WithMatchMember(memberNameOwnerChanged),
	)

	signalCh := make(chan *dbus.Signal)
	conn.Signal(signalCh)

	go func() {
		for {
			msg, ok := <-signalCh
			if !ok {
				log.Println("not ok")
				break
			}
			if msg.Name != signalNameOwnerChanged {
				continue
			}
			if len(msg.Body) != 3 {
				log.Printf("main: Object received didnt have enough args for %s. Wanted %d, got %d", signalNameOwnerChanged, 3, len(msg.Body))
				continue
			}
			if name, ok := msg.Body[0].(string); ok && mpris.HasValidDestinationName(name) {
				if ownerID, ok := msg.Body[2].(string); ok && ownerID != "" {
					log.Printf("Discovered new player: %s\n", name)
					player, err := mpris.NewPlayer(conn, name, ownerID, onDisconnect(&players, &model, &currentView), onPropertyChange(&players, &model, &currentView))
					if err != nil {
						log.Printf("Could not create a new player from %s: %s", name, err)
					}
					players = append(players, player)
				}
			}
		}
	}()
	obj := conn.Object(dbusDest, dbusObjectPath)

	/*obj2 := conn.Object("org.mpris.MediaPlayer2.spotify", "/org/mpris/MediaPlayer2")
	introspectResp, _ := introspect.Call(obj2)
	introspectJson, _ := json.MarshalIndent(introspectResp, "", "  ")
	log.Fatalln(string(introspectJson))
	*/
	resp := obj.Call("org.freedesktop.DBus.ListNames", dbus.Flags(0))
	if resp.Err != nil {
		log.Fatalf("listnames: %s", resp.Err)
	}

	var names []string
	if err := resp.Store(&names); err != nil {
		log.Fatalf("could not get names: %s", err)
	}

	for _, name := range names {
		if mpris.HasValidDestinationName(name) {
			var ownerID string
			ownerResp := obj.Call("org.freedesktop.DBus.GetNameOwner", 0, name)
			if err := ownerResp.Store(&ownerID); err != nil {
				log.Printf("Couldn't find owner for %s: %s", name, err)
			}
			player, err := mpris.NewPlayer(conn, name, ownerID, onDisconnect(&players, &model, &currentView), onPropertyChange(&players, &model, &currentView))
			if err != nil {
				log.Printf("Could not create a new player from %s: %s", name, err)
			}
			players = append(players, player)
		}
	}

	model.Options = showAllPlayers(players)
	model.Message = " "
	model.Render()

	for {
		v := <-eventCh

		selected, others := separatePlayers(players, v.Value)

		switch v.Cmd {
		case "pause":
			if err := selected.Pause(); err != nil {
				log.Printf("Could not pause (%s): %s", selected.Name, err)
			}

		case "play":
			for _, p := range others {
				if p.IsPlaying() {
					if err := p.Pause(); err != nil {
						log.Printf("Could not play (%s): %s", p.Name, err)
					}
				}
			}
			fallthrough
		case "playOne":
			if err := selected.Play(); err != nil {
				log.Printf("Could not play (%s): %s", selected.Name, err)
			}

		case "previous":
			if err := selected.Previous(); err != nil {
				log.Printf("Could not play previous track (%s): %s", selected.Name, err)
			}

		case "next":
			if err := selected.Next(); err != nil {
				log.Printf("Could not play next track (%s): %s", selected.Name, err)
			}

		case "controls":
			if selected != nil {
				model.Options = showControls(*selected, v)
				model.Message = formatControlMessage(*selected)
				model.Render()
				currentView = v
			}

		case "showAll":
			model.Options = showAllPlayers(players)
			model.Message = " "
			model.Render()
			currentView = rofi.Value{}

		default:
			return
		}
	}
}

func formatControlMessage(p mpris.Player) string {
	m := p.GetMetadata()
	title := ""

	if m.Title != "" {
		title += m.Title
		if m.Artist != "" {
			title = fmt.Sprintf("%s\r%s", html.EscapeString(title), html.EscapeString(m.Artist))
		}
		return title
	}

	if m.URL != "" {
		return title + path.Base(m.URL)
	}

	return p.Short
}

func separatePlayers(players []mpris.Player, name string) (*mpris.Player, []mpris.Player) {
	var selected *mpris.Player
	var others []mpris.Player
	for _, p := range players {
		if name == p.Name {
			disassociated := p
			selected = &disassociated
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
			Label: "Pause",
			Cmds:  []string{"pause"},
			Icon:  "player_pause",
			Value: v.Value,
		})
	} else {
		opts = append(opts, rofi.Option{
			Label: "Play",
			Cmds:  []string{"playOne"},
			Icon:  "player_play",
			Value: v.Value,
		})
	}

	opts = append(opts,
		rofi.Option{
			Label: "Previous",
			Cmds:  []string{"previous"},
			Icon:  "player_rew",
			Value: v.Value,
		},
		rofi.Option{
			Label: "Next",
			Cmds:  []string{"next"},
			Icon:  "player_fwd",
			Value: v.Value,
		},
		rofi.Option{
			Label: "Back",
			Cmds:  []string{"showAll"},
			Icon:  "back",
			Value: "",
		},
	)

	return opts
}

func showAllPlayers(players []mpris.Player) []rofi.Option {
	var opts []rofi.Option
	for _, player := range players {
		m := player.GetMetadata()

		title := formatTitle(m, player.Short, player.GetPlaybackStatus())
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
			Label: title,
			Icon:  icon,
			Value: player.Name,

			Category: category,
			Cmds:     []string{baseCmd, "controls"},

			IsMultiline: true,
			UseMarkup:   true,
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

func formatTitle(m mpris.Media, shortname string, playbackStatus mpris.PlaybackStatus) string {
	title := " "
	if playbackStatus == mpris.PlaybackStatusPlaying {
		title = " "
	} else if playbackStatus == mpris.PlaybackStatusStopped {
		title = " "
	}
	if m.Title != "" {
		title += m.Title
		if m.Artist != "" {
			title = fmt.Sprintf("%s\r%s", html.EscapeString(title), html.EscapeString(m.Artist))
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

func onDisconnect(players *[]mpris.Player, model *rofi.Model, view *rofi.Value) func(name string) {
	return func(name string) {
		for i, player := range *players {
			if player.Name == name {
				*players = append((*players)[:i], ((*players)[i+1:])...)
				switch view.Cmd {
				case "controls":
					selected, _ := separatePlayers(*players, view.Value)
					if selected.Name == name {
						model.Options = showAllPlayers(*players)
						model.Render()
					}

					model.Options = showControls(*selected, *view)
					model.Render()
				default:
					model.Options = showAllPlayers(*players)
					model.Render()
				}
			}
		}
	}
}

func onPropertyChange(players *[]mpris.Player, model *rofi.Model, view *rofi.Value) func(name string, changedProperties []string) {
	return func(name string, changedProperties []string) {
		switch view.Cmd {
		case "controls":
			selected, _ := separatePlayers(*players, view.Value)
			if selected != nil && selected.Name == name {
				model.Options = showControls(*selected, *view)
				model.Render()
			}
		default:
			model.Options = showAllPlayers(*players)
			model.Render()
		}
	}
}

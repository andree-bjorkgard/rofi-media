package mpris

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/godbus/dbus/v5"
)

type Player struct {
	obj         dbus.BusObject
	destination string
	Name        string
	Short       string

	CurrentTrack Track
}

var _ mediaPlayer2 = (*Player)(nil)
var _ mediaPlayer2Player = (*Player)(nil)

const objectPath = "/org/mpris/MediaPlayer2"

const rootInterfacePath = "org.mpris.MediaPlayer2."
const playerInterfacePath = "org.mpris.MediaPlayer2.Player."

var destinationRegexp *regexp.Regexp

var (
	errUnsupported        = errors.New("unsupported")
	errNotImplemented     = errors.New("not implemented")
	errInvalidDestination = errors.New("destination is not valid")
)

func init() {
	regexp, err := regexp.Compile("^org.mpris.MediaPlayer2.([a-zA-Z-_]{1}[a-zA-Z-_0-9.]*)$")
	if err != nil {
		log.Fatalf("player: Could not compile destination regexp. This should not occur: %s", err)
	}

	destinationRegexp = regexp
}

func HasValidDestinationName(destination string) bool {
	return destinationRegexp.MatchString(destination)
}

func NewPlayer(conn *dbus.Conn, dest string) (Player, error) {
	if !HasValidDestinationName(dest) {
		return Player{}, fmt.Errorf("player.NewPlayer: %w", errInvalidDestination)
	}
	short := destinationRegexp.FindStringSubmatch(dest)[1]

	o := conn.Object(dest, objectPath)

	return Player{
		obj:         o,
		destination: dest,
		Name:        dest,
		Short:       short,
	}, nil
}

func (p Player) getRootProp(prop string) (dbus.Variant, error) {
	return p.obj.GetProperty(rootInterfacePath + prop)
}

func (p Player) makeRootCall(method string, args ...any) error {
	call := p.obj.Call(rootInterfacePath+method, dbus.Flags(0), args...)
	return call.Err
}

func (p Player) getPlayerProp(prop string) (dbus.Variant, error) {
	return p.obj.GetProperty(playerInterfacePath + prop)
}

func (p Player) makePlayerCall(method string, args ...any) error {
	call := p.obj.Call(playerInterfacePath+method, dbus.Flags(0), args...)
	return call.Err
}

func (p Player) CanRaise() bool {
	prop, err := p.getRootProp("CanRaise")
	if err != nil {
		return false
	}

	if v, ok := prop.Value().(bool); !ok || ok && !v {
		return false
	}

	return true
}

func (p Player) Raise() error {
	if !p.CanRaise() {
		return fmt.Errorf("mpris.Raise: %s", errUnsupported)
	}

	err := p.makeRootCall("Raise")
	if err != nil {
		return fmt.Errorf("mpris.Raise: %w", err)
	}

	return nil
}

func (p Player) CanQuit() bool {
	prop, err := p.getRootProp("CanQuit")
	if err != nil {
		return false
	}

	if v, ok := prop.Value().(bool); !ok || ok && !v {
		return false
	}

	return true
}

func (p Player) Quit() error {
	if !p.CanQuit() {
		return fmt.Errorf("mpris.Quit: %s", errUnsupported)
	}

	err := p.makeRootCall("Quit")
	if err != nil {
		return fmt.Errorf("mpris.Quit: %w", err)
	}

	return nil
}

func (p Player) CanPlay() bool {
	prop, err := p.getPlayerProp("CanPlay")
	if err != nil {
		return false
	}

	if v, ok := prop.Value().(bool); !ok || ok && !v {
		return false
	}

	return true
}

func (p Player) Play() error {
	if !p.CanPlay() {
		return fmt.Errorf("mpris.Play: %s", errUnsupported)
	}

	err := p.makePlayerCall("Play")

	if err != nil {
		return fmt.Errorf("mpris.Play: %w", err)
	}

	return nil
}

func (p Player) CanControl() bool {
	prop, err := p.getPlayerProp("CanControl")
	if err != nil {
		return false
	}

	if v, ok := prop.Value().(bool); !ok || ok && !v {
		return false
	}

	return true
}

func (p Player) Stop() error {
	if !p.CanControl() {
		return fmt.Errorf("mpris.Stop: %s", errUnsupported)
	}

	err := p.makePlayerCall("Stop")

	if err != nil {
		return fmt.Errorf("mpris.Stop: %w", err)
	}

	return nil
}

func (p Player) CanPause() bool {
	prop, err := p.getPlayerProp("CanPause")
	if err != nil {
		return false
	}

	if v, ok := prop.Value().(bool); !ok || ok && !v {
		return false
	}

	return true
}

func (p Player) Pause() error {
	if !p.CanPause() {
		return fmt.Errorf("mpris.Pause: %s", errUnsupported)
	}

	err := p.makePlayerCall("Pause")

	if err != nil {
		return fmt.Errorf("mpris.Pause: %w", err)
	}

	return nil
}

func (p Player) PlayPause() error {
	if !p.CanPause() || !p.CanPlay() {
		return fmt.Errorf("mpris.PlayPause: %s", errUnsupported)
	}

	err := p.makePlayerCall("PlayPause")

	if err != nil {
		return fmt.Errorf("mpris.PlayPause: %w", err)
	}

	return nil
}

func (p Player) CanGoNext() bool {
	prop, err := p.getPlayerProp("CanGoNext")
	if err != nil {
		return false
	}

	if v, ok := prop.Value().(bool); !ok || ok && !v {
		return false
	}

	return true
}

func (p Player) Next() error {
	if !p.CanGoNext() {
		return fmt.Errorf("mpris.Next: %s", errUnsupported)
	}

	err := p.makePlayerCall("Next")

	if err != nil {
		return fmt.Errorf("mpris.Next: %w", err)
	}

	return nil
}

func (p Player) CanGoPrevious() bool {
	prop, err := p.getPlayerProp("CanGoPrevious")
	if err != nil {
		return false
	}

	if v, ok := prop.Value().(bool); !ok || ok && !v {
		return false
	}

	return true
}

func (p Player) Previous() error {
	if !p.CanGoPrevious() {
		return fmt.Errorf("mpris.Previous: %s", errUnsupported)
	}

	err := p.makePlayerCall("Previous")

	if err != nil {
		return fmt.Errorf("mpris.Previous: %w", err)
	}

	return nil
}

func (p Player) CanSeek() bool {
	prop, err := p.getPlayerProp("CanSeek")
	if err != nil {
		return false
	}

	if v, ok := prop.Value().(bool); !ok || ok && !v {
		return false
	}

	return true
}

func (p Player) Seek(seconds int) error {
	if !p.CanSeek() {
		return fmt.Errorf("mpris.Seek: %s", errUnsupported)
	}

	err := p.makePlayerCall("Seek", (time.Duration(seconds) * time.Second).Microseconds())

	if err != nil {
		return fmt.Errorf("mpris.Seek: %w", err)
	}

	return nil
}

func (p Player) GetMetadata() (Track, error) {
	if p.CurrentTrack.Title == "" {
		m, err := p.getPlayerProp("Metadata")
		if err != nil {
			return p.CurrentTrack, fmt.Errorf("mpris.GetMetadata: Could not get metadata: %s", err)
		}

		track, err := decodeMetadata(m)
		if err != nil {
			return p.CurrentTrack, fmt.Errorf("mpris.GetMetadata: Could not decode metadata: %s", err)
		}

		p.CurrentTrack = track
	}

	return p.CurrentTrack, nil
}

func (p Player) GetPlaybackStatus() (PlaybackStatus, error) {
	v, err := p.getPlayerProp("PlaybackStatus")
	if err != nil {
		return PlaybackStatus(""), err
	}

	val, ok := v.Value().(string)
	if !ok {
		return PlaybackStatus(""), fmt.Errorf("mpris.GetPlaybackStatus: Not a valid type, expected string, got: %T ", v.Value())
	}

	status := PlaybackStatus(val)
	if !status.IsValid() {
		return status, fmt.Errorf("mpris.GetPlaybackStatus: Not a valid playbackstatus, got: %s", status)
	}

	return status, nil
}

func (p Player) IsPlaying() bool {
	s, _ := p.GetPlaybackStatus()

	return s == PlaybackStatusPlaying
}

func (p *Player) SetPosition(trackID dbus.ObjectPath, microseconds int64) error {
	return errNotImplemented
}

func (p *Player) OpenUri(uri string) error {
	return errNotImplemented
}

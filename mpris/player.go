package mpris

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

type Player struct {
	obj         dbus.BusObject
	destination string
	Name        string
	Short       string
	ownerID     string

	properties  *properties
	isConnected bool
}

var _ mediaPlayer2 = (*Player)(nil)
var _ mediaPlayer2Player = (*Player)(nil)

const (
	objectPathMpris = "/org/mpris/MediaPlayer2"
	objectPathDBus  = "/org/freedesktop/DBus"

	interfacePathMprisMediaPlayer2       = "org.mpris.MediaPlayer2"
	interfacePathMprisMediaPlayer2Player = "org.mpris.MediaPlayer2.Player"
	interfacePathDBusProperties          = "org.freedesktop.DBus.Properties"
	interfacePathDBus                    = "org.freedesktop.DBus"

	memberNameOwnerChanged      = "NameOwnerChanged"
	memberNamePropertiesChanged = "PropertiesChanged"

	signalNamePropertiesChanged = interfacePathDBusProperties + "." + memberNamePropertiesChanged
	signalNameOwnerChanged      = interfacePathDBus + "." + memberNameOwnerChanged
)

var destinationRegexp *regexp.Regexp

var (
	ErrUnsupported        = errors.New("unsupported")
	ErrNotImplemented     = errors.New("not implemented")
	ErrInvalidDestination = errors.New("destination is not valid")
)

func init() {
	regexp, err := regexp.Compile("^org.mpris.MediaPlayer2.([a-zA-Z-_]{1}[a-zA-Z-_0-9.]*)$")
	if err != nil {
		log.Fatalf("mpris: Could not compile destination regexp: %s", err)
	}

	destinationRegexp = regexp
}

func HasValidDestinationName(destination string) bool {
	return destinationRegexp.MatchString(destination)
}

type PlayerEvent string

const (
	PlayerEventPropertyChange = "PropertyChanged"
	PlayerEventDisconnected   = "Disconnected"
)

type PlayerEventMessage struct {
	Event PlayerEvent
	Name  string
}

func (p *Player) Register(c *dbus.Conn, onDisconnect func(name string), onPropertyChange func(name string, changedProps []string)) {
	signalPropertyChangeOpts := []dbus.MatchOption{
		dbus.WithMatchObjectPath(objectPathMpris),
		dbus.WithMatchInterface(interfacePathDBusProperties),
		dbus.WithMatchMember(memberNamePropertiesChanged),
		dbus.WithMatchSender(p.ownerID),
	}

	err := c.AddMatchSignal(signalPropertyChangeOpts...)
	if err != nil {
		log.Printf("mpris.listener: Could not listen on property changes for %s: %s", p.destination, err)
	}

	signalNameOwnerChangedOpts := []dbus.MatchOption{
		dbus.WithMatchObjectPath(objectPathDBus),
		dbus.WithMatchInterface(interfacePathDBus),
		dbus.WithMatchMember(memberNameOwnerChanged),
		dbus.WithMatchSender(p.ownerID),
	}
	err = c.AddMatchSignal(signalNameOwnerChangedOpts...)
	if err != nil {
		log.Printf("mpris.listener: Could not listen on disconnect changes for %s: %s", p.destination, err)
	}

	signalCh := make(chan *dbus.Signal)
	c.Signal(signalCh)

	go func(signalCh chan *dbus.Signal, conn *dbus.Conn) {
		defer func() {
			defer close(signalCh)
			c.RemoveSignal(signalCh)
			onDisconnect(p.Name)
			conn.RemoveMatchSignal(signalNameOwnerChangedOpts...)
			conn.RemoveMatchSignal(signalPropertyChangeOpts...)
		}()

	Loop:
		for {
			msg, ok := <-signalCh
			if !ok {
				break
			}

			switch msg.Name {
			case signalNamePropertiesChanged:
				if msg.Sender != p.ownerID {
					continue
				}
				if len(msg.Body) != 3 {
					log.Printf("mpris.Register: Object received didnt have enough args for %s. Wanted %d, got %d", signalNamePropertiesChanged, 3, len(msg.Body))
				}
				if msg.Body[0] == interfacePathMprisMediaPlayer2Player {
					varMap, ok := msg.Body[1].(map[string]dbus.Variant)
					if !ok {
						log.Printf("mpris.Register: Object received didnt have a valid body for %s. Got %v", signalNamePropertiesChanged, varMap)
					}

					if cl := p.UpdateProperties(varMap); len(cl) > 0 {
						onPropertyChange(p.Name, cl)
					}
				}

			case signalNameOwnerChanged:
				if name, ok := msg.Body[0].(string); ok && name == p.Name {
					if ownerID, ok := msg.Body[1].(string); ok && ownerID != "" {
						log.Printf("Player disconnected: %s\n", name)
						break Loop
					}
				}
			}
		}
	}(signalCh, c)
}

type properties struct {
	PlaybackStatus PlaybackStatus
	LoopStatus     LoopStatus
	Shuffle        bool

	Media Media

	sync.Mutex
}

func NewPlayer(conn *dbus.Conn, dest string, ownerID string, onDisconnect func(name string), onPropertyChange func(name string, changedProps []string)) (Player, error) {
	var player Player
	if !HasValidDestinationName(dest) {
		return player, fmt.Errorf("player.NewPlayer: %w", ErrInvalidDestination)
	}
	short := destinationRegexp.FindStringSubmatch(dest)[1]

	o := conn.Object(dest, objectPathMpris)

	player = Player{
		obj:         o,
		destination: dest,
		ownerID:     ownerID,
		Name:        dest,
		Short:       short,
	}

	call := o.Call(interfacePathDBusProperties+".GetAll", 0, interfacePathMprisMediaPlayer2Player)
	if call.Err != nil {
		return player, fmt.Errorf("mpris.NewPlayer: Error while calling to get all properties on %s: %w", dest, call.Err)
	}

	var rawProps map[string]dbus.Variant
	if err := call.Store(&rawProps); err != nil {
		return player, fmt.Errorf("mpris.NewPlayer: Error while decoding all properties on %s: %w", dest, call.Err)
	}

	player.UpdateProperties(rawProps)

	player.Register(conn, onDisconnect, onPropertyChange)

	player.isConnected = true
	return player, nil
}

func (p *Player) UpdateProperties(props map[string]dbus.Variant) (changeList []string) {
	if p.properties == nil {
		p.properties = &properties{}
	}

	p.properties.Lock()
	defer p.properties.Unlock()

	for key, val := range props {
		switch key {
		case "Shuffle":
			if v, ok := val.Value().(bool); ok && p.properties.Shuffle != v {
				p.properties.Shuffle = v
				changeList = append(changeList, key)
			}

		case "PlaybackStatus":
			if v, ok := val.Value().(string); ok {
				s := PlaybackStatus(v)
				if s.IsValid() && p.properties.PlaybackStatus != s {
					p.properties.PlaybackStatus = s
					changeList = append(changeList, key)
				}
			}

		case "LoopStatus":
			if v, ok := val.Value().(string); ok {
				s := LoopStatus(v)
				if !s.IsValid() {
					s = LoopStatusNone
				}
				if p.properties.LoopStatus != s {
					p.properties.LoopStatus = s
					changeList = append(changeList, key)
				}
			}

		case "Metadata":
			if err := decodeMetadata(val.Value(), &p.properties.Media); err == nil {
				changeList = append(changeList, key)
			}

		default:
			continue
		}
	}

	return
}

func (p Player) getRootProp(prop string) (dbus.Variant, error) {
	return p.obj.GetProperty(interfacePathMprisMediaPlayer2 + "." + prop)
}

func (p Player) makeRootCall(method string, args ...any) error {
	call := p.obj.Call(interfacePathMprisMediaPlayer2+"."+method, dbus.Flags(0), args...)
	return call.Err
}

func (p Player) getPlayerProp(prop string) (dbus.Variant, error) {
	return p.obj.GetProperty(interfacePathMprisMediaPlayer2Player + "." + prop)
}

func (p Player) makePlayerCall(method string, args ...any) error {
	call := p.obj.Call(interfacePathMprisMediaPlayer2Player+"."+method, dbus.Flags(0), args...)
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
		return fmt.Errorf("mpris.Raise: %s", ErrUnsupported)
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
		return fmt.Errorf("mpris.Quit: %s", ErrUnsupported)
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
	if p.properties.PlaybackStatus == PlaybackStatusPlaying {
		return nil
	}
	if !p.CanPlay() {
		return fmt.Errorf("mpris.Play: %s", ErrUnsupported)
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
	if p.properties.PlaybackStatus == PlaybackStatusStopped {
		return nil
	}

	if !p.CanControl() {
		return fmt.Errorf("mpris.Stop: %s", ErrUnsupported)
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
	if p.properties.PlaybackStatus == PlaybackStatusPaused {
		return nil
	}

	if !p.CanPause() {
		return fmt.Errorf("mpris.Pause: %s", ErrUnsupported)
	}

	err := p.makePlayerCall("Pause")

	if err != nil {
		return fmt.Errorf("mpris.Pause: %w", err)
	}

	return nil
}

func (p Player) PlayPause() error {
	if !p.CanPause() || !p.CanPlay() {
		return fmt.Errorf("mpris.PlayPause: %s", ErrUnsupported)
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
		return fmt.Errorf("mpris.Next: %s", ErrUnsupported)
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
		return fmt.Errorf("mpris.Previous: %s", ErrUnsupported)
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
		return fmt.Errorf("mpris.Seek: %s", ErrUnsupported)
	}

	err := p.makePlayerCall("Seek", (time.Duration(seconds) * time.Second).Microseconds())

	if err != nil {
		return fmt.Errorf("mpris.Seek: %w", err)
	}

	return nil
}

func (p Player) GetMetadata() Media {
	return p.properties.Media
}

func (p Player) GetPlaybackStatus() PlaybackStatus {
	return p.properties.PlaybackStatus
}

func (p Player) IsPlaying() bool {
	s := p.GetPlaybackStatus()

	return s == PlaybackStatusPlaying
}

func (p *Player) SetPosition(trackID dbus.ObjectPath, microseconds int64) error {
	return ErrNotImplemented
}

func (p *Player) OpenUri(uri string) error {
	return ErrNotImplemented
}

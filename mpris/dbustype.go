package mpris

import "github.com/godbus/dbus/v5"

/*
type Y uint8
type B bool
type N int16
type Q uint16
type I int32
type U uint32
type X int64
type T uint64
type D float64
type H uint32

type S string

// OBJECT_PATH
type O string

// SIGNATURE
type G string
*/

type mediaPlayer2 interface {
	Raise() error
	Quit() error
}

type mediaPlayer2Player interface {
	Next() error
	Previous() error
	Pause() error
	PlayPause() error
	Stop() error
	Play() error
	Seek(seconds int) error
	SetPosition(trackId dbus.ObjectPath, microseconds int64) error
	OpenUri(uri string) error
}

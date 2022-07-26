package mpris

type PlaybackStatus string

const (
	PlaybackStatusPaused  PlaybackStatus = "Paused"
	PlaybackStatusPlaying PlaybackStatus = "Playing"
	PlaybackStatusStopped PlaybackStatus = "Stopped"
)

func (e PlaybackStatus) IsValid() bool {
	switch e {
	case PlaybackStatusPaused, PlaybackStatusPlaying, PlaybackStatusStopped:
		return true
	}
	return false
}

func (e PlaybackStatus) String() string {
	return string(e)
}

type LoopStatus string

const (
	LoopStatusNone     LoopStatus = "None"
	LoopStatusTrack    LoopStatus = "Track"
	LoopStatusPlaylist LoopStatus = "Playlist"
)

func (e LoopStatus) IsValid() bool {
	switch e {
	case LoopStatusNone, LoopStatusTrack, LoopStatusPlaylist:
		return true
	}
	return false
}

func (e LoopStatus) String() string {
	return string(e)
}

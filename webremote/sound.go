package main

type soundState struct {
	Night      *bool   `json:"night,omitempty"`
	Bass       *uint32 `json:"bass,omitempty"`
	Atmos      *uint32 `json:"atmos,omitempty"`
	Rear       *uint32 `json:"rear,omitempty"`
	RearDocked *bool   `json:"rear_docked,omitempty"`
}

func (d *device) sound() soundState {
	var s soundState
	if v, e := d.read(0xe4329a, 1); e == nil && v <= 1 {
		on := v == 1
		s.Night = &on
	}
	if v, e := d.configByte(238); e == nil && v <= 4 {
		s.Bass = &v
	}
	if v, e := d.configByte(201); e == nil && v <= 2 {
		s.Atmos = &v
	}
	if v, e := d.configByte(202); e == nil && v <= 3 {
		s.Rear = &v
	}
	left, le := d.read(0xe435ec, 1)
	right, re := d.read(0xe435ed, 1)
	if le == nil && re == nil && left <= 1 && right <= 1 {
		docked := left != 0 || right != 0
		s.RearDocked = &docked
	}
	return s
}
func soundValue(action string, s *soundState) (uint32, uint32, bool) {
	if s == nil {
		return 0, 0, false
	}
	var value *uint32
	var count uint32
	switch action {
	case "bass_next":
		value = s.Bass
		count = 5
	case "atmos_next":
		value = s.Atmos
		count = 3
	case "rear_next":
		value = s.Rear
		count = 4
	}
	if value == nil || *value >= count {
		return 0, 0, false
	}
	return *value, count, true
}
func soundObserved(action string, before, after *soundState) bool {
	old, count, ok := soundValue(action, before)
	next, _, found := soundValue(action, after)
	return ok && found && next == (old+1)%count
}

type streamState struct {
	native  uint32
	Detail  string        `json:"detail,omitempty"`
	Cast    *CastSnapshot `json:"cast,omitempty"`
	Service string        `json:"service"`
	Status  string        `json:"status"`
	Active  bool          `json:"active"`
}

func sourceName(value uint32) string {
	switch value {
	case 0:
		return "TV / ARC"
	case 1:
		return "Optical"
	case 2:
		return "Bluetooth"
	case 3:
		return "HDMI"
	case 4:
		return "AUX"
	case 5:
		return "USB"
	case 6:
		return "Chromecast"
	case 7:
		return "AirPlay"
	}
	return "Unknown"
}
func (d *device) streaming(source uint32) *streamState {
	switch source {
	case 6:
		native, _ := d.read(0xe30710, 4)
		return castStream(native, cachedCastStatus())
	case 7:
		s := &streamState{Service: "AirPlay", Status: "Selected"}
		v, e := d.read(0xe3169c, 4)
		if e == nil && v == 1 {
			s.Status = "Session active"
			s.Active = true
		}
		return s
	}
	return nil
}

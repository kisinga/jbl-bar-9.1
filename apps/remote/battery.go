package main

// rearBattery reads the UART-reported percentage on the verified firmware.
// Native BATL/R display routine 0x7f3e60 reads 0xe435ec + 0xd / + 0xf.
// Values above 100 encode the percentage modulo 100, with zero meaning 100.
// Unlike the native display, preserve 90..99 rather than rounding to 100.
// This is cached telemetry, not proof of current connection or charging.
func (d *device) rearBattery(address int64) *uint32 {
	v, err := d.read(address, 1)
	if err != nil {
		return nil
	}
	return rearBatteryPercent(v)
}

func rearBatteryPercent(raw uint32) *uint32 {
	// Only the plain and +100 ranges are qualified; reject other/sentinel bytes.
	if raw > 200 {
		return nil
	}
	if raw > 100 {
		raw %= 100
		if raw == 0 {
			raw = 100
		}
	}
	return &raw
}

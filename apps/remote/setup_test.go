package main

import "testing"

func TestCalibrationPhases(t *testing.T) {
	cases := map[uint32]string{
		msgCalibration1: "Step 1 starting", msgCalibration2: "Step 2 starting", 64: "Starting in 5", 60: "Starting in 1",
		msgCalibrating: "Measuring", msgCalDone1: "Step 1 done", msgCalDone2: "Step 2 done",
		msgCalRetry: "Failed, retry", msgCalCancelled: "Cancelled", vfdMessageNone: "", 21: "",
	}
	for message, want := range cases {
		if got := calibrationPhase(message); got != want {
			t.Fatalf("message %d: got %q want %q", message, got, want)
		}
	}
	for message := uint32(0); message <= vfdMessageNone; message++ {
		want := message >= msgCalibration1 && message <= msgCalibrating
		if calibrationRunning(message) != want {
			t.Fatalf("running(%d)", message)
		}
	}
}

func TestSubwooferMessages(t *testing.T) {
	if subwooferMessage(msgSubConnected) != "Connected" || subwooferMessage(msgSubNotConnected) != "Not connected" ||
		subwooferMessage(msgReconnectSub) != "Reconnecting" || subwooferMessage(vfdMessageNone) != "" {
		t.Fatal("subwoofer messages")
	}
}

func TestAudioFormatText(t *testing.T) {
	b := make([]byte, vfdAudioLength)
	copy(b, "DOLBY SURROUND DD+")
	if got := audioFormatText(b); got != "DOLBY SURROUND DD+" {
		t.Fatalf("got %q", got)
	}
	copy(b, "  PCM  \x00garbage")
	if got := audioFormatText(b); got != "PCM" {
		t.Fatalf("got %q", got)
	}
	if audioFormatText([]byte{0xff, 'A', 0}) != "" || audioFormatText(make([]byte, 8)) != "" {
		t.Fatal("non-text accepted")
	}
}

func TestAudioFormatFollowsTheSource(t *testing.T) {
	tv, hdmi, bt := uint32(0), uint32(3), uint32(2)
	var f formatTracker
	if f.observe("DOLBY ATMOS DD+", false, &hdmi) != "DOLBY ATMOS DD+" {
		t.Fatal("format on HDMI")
	}
	if f.observe("DOLBY ATMOS DD+", false, &bt) != "" {
		t.Fatal("Bluetooth must not report the previous HDMI format")
	}
	if f.observe("DOLBY ATMOS DD+", false, &tv) != "" {
		t.Fatal("TV must not report a format shown on HDMI")
	}
	if f.observe("DOLBY ATMOS DD+", true, &tv) != "DOLBY ATMOS DD+" {
		t.Fatal("same format shown again on TV")
	}
	if f.observe("PCM", false, &tv) != "PCM" {
		t.Fatal("new format shown on TV")
	}
	if f.observe("PCM", false, &hdmi) != "" || f.observe("PCM", false, nil) != "" {
		t.Fatal("stale or unknown source")
	}
}

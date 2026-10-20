package main

import "testing"

func TestBluetoothRPCNeedsBothKeyEdgesAndCleanClose(t *testing.T) {
	good := `{"check_only":false,"init_result":0,"connected":true,"close_result":0,"press_result":0,"release_result":0,"callbacks_registered":false}`
	if !bluetoothRPCSucceeded([]byte(good)) {
		t.Fatal("complete reply rejected")
	}
	for _, b := range []string{
		`{}`, `null`, good + `{}`,
		`{"init_result":0,"connected":true,"close_result":0,"press_result":0}`,
		`{"init_result":0,"connected":true,"close_result":0,"press_result":0,"release_result":-1}`,
		`{"init_result":0,"connected":true,"close_result":-1,"press_result":0,"release_result":0}`,
		`{"init_result":0,"connected":false,"close_result":0,"press_result":0,"release_result":0}`,
		`{"init_result":0,"connected":true,"close_result":0,"press_result":0,"release_result":0,"callbacks_registered":true}`,
		`{"check_only":true,"init_result":0,"connected":true,"close_result":0,"press_result":0,"release_result":0}`,
	} {
		if bluetoothRPCSucceeded([]byte(b)) {
			t.Fatalf("uncertain result accepted: %s", b)
		}
	}
}

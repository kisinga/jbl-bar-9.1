package main

import (
	"context"
	"errors"
	"strings"
)

// An applied Front EQ is reset before native controls change the sound. The
// reset is only qualified on the Front EQ route (Standard playback, HPP page
// 0x840000). The bar often leaves that route by itself: Smart Mode returns
// after wake, and HDMI Dolby uses the film page 0x840002. There the native
// processing mode is no longer the custom one (observed on 4 Nov 2026: mode 2,
// cache 2 on page 0x840002 in Smart Mode), so the custom EQ no longer plays,
// yet the pending reset blocked every control, including Standard, which is
// needed to return to the qualified route.
//
// When the full reader rejects the route, the narrow mode reader decides:
//   - not the custom mode: controls proceed; the reset stays pending and the
//     EQ monitor confirms it (mode, preset and baseline) once the qualified
//     route returns;
//   - custom mode still active: changing input or selecting Standard proceeds,
//     since both lead back to a route where Reset works; other controls are
//     refused with what to do.
//
// Any other reset failure keeps refusing the control, as before.
const eqCustomMode = 5

const (
	eqDeferredProblem = "Custom EQ is not active on this input. The original tuning is confirmed automatically when Standard Bluetooth playback returns."
	eqActiveProblem   = "Custom EQ is still active on this input. Select Standard and play Bluetooth audio, then select Reset in Front EQ."
)

func eqPendingDecision(resetErr error, action string, mode func() (uint32, error)) (proceed bool, problem string, err error) {
	if resetErr == nil {
		return true, "", nil
	}
	if !errors.Is(resetErr, errEQRoute) {
		return false, "", resetErr
	}
	if m, e := mode(); e == nil && m != eqCustomMode {
		return true, eqDeferredProblem, nil
	}
	if strings.HasPrefix(action, "input_") || action == "standard" {
		return true, eqActiveProblem, nil
	}
	return false, eqActiveProblem, errors.New(eqActiveProblem)
}

// resetFrontEQBefore resets an applied Front EQ before a native control, or
// lets the control proceed as eqPendingDecision allows. Callers hold d.mu.
func (d *device) resetFrontEQBefore(ctx context.Context, action string) error {
	if !d.eq.active {
		return nil
	}
	proceed, problem, err := eqPendingDecision(d.resetFrontEQ(ctx), action, d.readHPPMode)
	if problem != "" {
		d.eq.problem = problem
	}
	if !proceed {
		return err
	}
	return nil
}

// eqPendingProblem describes a reset the EQ monitor could not finish.
func eqPendingProblem(resetErr error, mode func() (uint32, error)) string {
	if errors.Is(resetErr, errEQRoute) {
		if m, e := mode(); e == nil && m != eqCustomMode {
			return eqDeferredProblem
		}
		return eqActiveProblem
	}
	return "EQ reset pending: " + resetErr.Error()
}

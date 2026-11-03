package main

import (
	"context"
	"jbl-bar-toolkit/remote/internal/server"
)

// Thin compatibility methods preserve native feature callers and fixtures.
type stateFeed struct {
	server.Feed[state]
	decorate func(state) state
}

func (f *stateFeed) subscribe() (chan state, func()) { return f.Subscribe() }
func (f *stateFeed) publish(s state) {
	if f.decorate != nil {
		s = f.decorate(s)
	}
	f.Publish(s)
}
func (f *stateFeed) run(ctx context.Context, snapshot func() state) { f.Run(ctx, snapshot) }
func (s state) StateRevision() uint64                               { return s.Revision }
func (s state) PublicState(token string) any                        { s.CSRF = token; return s }
func (s state) ChangeState() any                                    { s.Uptime = 0; s.Revision = 0; s.CSRF = ""; return s }

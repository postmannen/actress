package actress

import "context"

// --- ETRemote function to be used by both nodes
// remoteCh := make(chan Event)

const ETChannelWrite EventName = "ETChannelWrite"

func ETChannelWriteFn(remoteCh chan Event) ETFunc {
	fn := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case ev := <-p.InCh:
					// The event came here via the ETRemote Actor. The event we
					// got here is an ETChannelWrite event, but the ETRemote
					// actor put the actual event to forward as the NextEvent of the
					// ETChannelWrite, so we just put the NextEvent on the channel.
					nextEV := ev.NextEvent
					remoteCh <- *nextEV

				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}
	return fn
}

const ETChannelRead EventName = "ETChannelRead"

func ETChannelReadFn(remoteCh chan Event) ETFunc {
	fn := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case ev := <-remoteCh:
					// Take the event we received at the remote destination side, and just add
					// the event to be processed locally.
					p.AddEvent(ev)
				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}
	return fn
}

func ETRemoteChannelReadWriteFn() ETFunc {
	fn := func(ctx context.Context, p *Process) func() {
		fn := func() {
			p.SignalReady()

			for {
				select {
				case ev := <-p.InCh:
					nextEv := ev.NextEvent
					// The src node is set in the event, so the remote will know
					// where it came from if it at some point would like to
					// repply on this. This is not used for this test, just explaining
					// here for how to use ETRemote.
					nextEv.SrcNode = p.Config.NodeName

					evToSend := Event{Name: ETChannelWrite, NextEvent: nextEv}
					p.AddEvent(evToSend)

				case <-ctx.Done():
				}
			}
		}

		return fn
	}
	return fn
}

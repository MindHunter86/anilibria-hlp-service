package app

import (
	"net/netip"
	"sync"
	"time"
)

type blocklist struct {
	locker sync.RWMutex
	list   map[netip.Addr]time.Time

	ticklock sync.Mutex
	ticker   *time.Ticker
	wg       sync.WaitGroup

	isEnabled bool
}

func newBlocklist(enabled ...bool) *blocklist {
	enabled = append(enabled, false) // add default

	return &blocklist{
		list:      make(map[netip.Addr]time.Time),
		isEnabled: enabled[0],
	}
}

func (m *blocklist) run(done func()) {
	defer done()

	if !m.isEnabled {
		return
	}

	log.Debug().Msg("strating blocklist loop...")
	m.ticker = time.NewTicker(time.Minute)

loop:
	for {
		select {
		case <-gCtx.Done():
			log.Warn().Msg("context done() has been caught; stopping cron subservice...")
			break loop
		case <-m.ticker.C:
			log.Trace().Msg("start searching of expired bans...")

			m.wg.Add(1)
			m.findExpiredBans(m.wg.Done)
		}
	}

	m.ticker.Stop()

	log.Debug().Msg("waiting for goroutines with blocklist jobs...")
	m.wg.Wait()
}

func (m *blocklist) findExpiredBans(done func()) {
	defer done()

	if !m.ticklock.TryLock() {
		log.Debug().Msg("could not get lock for the job, seems previous call has not been completed yet")
		return
	}
	defer m.ticklock.Unlock()

	m.locker.RLock()

	blist := m.list
	m.locker.RUnlock()

	now := time.Now()
	for addr, time := range blist {
		if time.Before(now) {
			log.Debug().Str("ip", addr.String()).Time("now", now).Time("ban_time", time).Msg("unblocking ip address...")
			m.pop(addr)
		}
	}
}

func (m *blocklist) push(ip []byte) bool {
	if !m.isEnabled {
		return true
	}

	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return ok
	}

	m.locker.RLock()

	m.list[addr] = time.Now().Add(ccx.Duration("ip-ban-time"))
	m.locker.RUnlock()

	log.Debug().Str("addr", addr.String()).Time("ban_time", time.Now().Add(ccx.Duration("ip-ban-time"))).
		Msg("ip addres has been added to blocklist")
	return ok
}

func (m *blocklist) pop(addr netip.Addr) {
	if !m.isEnabled {
		return
	}

	m.locker.Lock()

	delete(m.list, addr)
	m.locker.Unlock()

	log.Debug().Str("addr", addr.String()).Msg("ip addres has been remove from blocklist")
}

func (m *blocklist) isExists(ip []byte) (ok bool) {
	if !m.isEnabled {
		return false
	}

	var addr netip.Addr
	if addr, ok = netip.AddrFromSlice(ip); !ok {
		return false
	}

	m.locker.RLock()

	_, ok = m.list[addr]
	m.locker.RUnlock()

	return
}
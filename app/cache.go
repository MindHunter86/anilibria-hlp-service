package app

import (
	"errors"
	"sync"

	"github.com/rs/zerolog"
)

type CachedTitlesBucket struct {
	locker sync.RWMutex
	bucket map[uint32]*TitleSerie
}

func NewCachedTitlesBucket() *CachedTitlesBucket {
	return &CachedTitlesBucket{
		bucket: make(map[uint32]*TitleSerie),
	}
}

func (m *CachedTitlesBucket) PullSerie(tid, sid uint16) (serie *TitleSerie, _ error) {
	if tid == 0 || sid == 0 {
		return serie, errors.New("cache: tid, sid zero found")
	}

	m.locker.RLock()
	if zerolog.GlobalLevel() <= zerolog.DebugLevel {
		log.Debug().Int("cache_size", len(m.bucket)+1).Msg("cache size debug")
	}

	serie = m.bucket[uint32(tid)<<16|uint32(sid)]
	m.locker.RUnlock()

	return
}

func (m *CachedTitlesBucket) PushSerie(serie *TitleSerie) (_ error) {
	if serie == nil || serie.Title == 0 || serie.Serie == 0 {
		return errors.New("cache: serie.Title, serie.Serie zero found or serie is undefined")
	}

	m.locker.Lock()

	m.bucket[uint32(serie.Title)<<16|uint32(serie.Serie)] = serie
	m.locker.Unlock()

	return
}

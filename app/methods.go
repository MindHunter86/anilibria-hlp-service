package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type (
	Title struct {
		// Names      interface{}
		// Updated    uint64
		// LastChange uint64 `json:"last_change"`

		Id     uint16
		Code   string
		Player *Player
	}
	Player struct {
		// AlternativePlayer string `json:"alternative_player"`
		// Series            interface{}

		Host     string
		Playlist map[string]*PlayerPlaylist
	}
	PlayerPlaylist struct {
		// CreatedTimestamp uint64 `json:"created_timestamp"`
		// Preview          string
		// Skips            interface{}

		Serie uint16
		Hls   *PlaylistHls
	}
	PlaylistHls struct {
		Fhd, Hd, Sd string
	}
)

type (
	titleQuality uint8
	TitleSerie   struct {
		Title, Serie  uint16
		QualityHashes map[titleQuality]string
	}
)

const (
	titleQualityNone titleQuality = iota
	titleQualitySD
	titleQualityHD
	titleQualityFHD
)

func (m *titleQuality) string() string {
	switch *m {
	case titleQualitySD:
		return "480"
	case titleQualityHD:
		return "720"
	case titleQualityFHD:
		return "1080"
	default:
		return ""
	}
}

// redis schema
// https://cache.libria.fun/videos/media/ts/9277/13/720/3ae5aa5839690b8d9ea9fcef9b720fb4_00028.ts
// https://cache.libria.fun/videos/media/ts/9222/11/1080/97d3bb428727bc25fa110bc51826a366.m3u8
// KEY - VALUE
// ID:SERIE - FHD:HD:SD (links)
// ID:SERIE:{FHD,HD,SD} - json{[TS LINKS]}

const (
	tsrRawTitleId = uint8(iota) + 4 // 4 is a skipping of "/videos/media/ts" parts
	tsrRawTitleSerie
	tsrRawTitleQuality
	tsrRawFilename
)

func getHashFromUriPath(upath string) (hash string, ok bool) {
	switch upath[len(upath)-1:] {
	case "s": // .ts
		if hash, _, ok = strings.Cut(upath, ".ts"); !ok {
			return "", ok
		}

		if hash, _, ok = strings.Cut(hash, "_"); !ok {
			return "", ok
		}
	case "8": // .m3u8
		if hash, _, ok = strings.Cut(upath, ".m3u8"); !ok {
			return "", ok
		}
	default:
		return "", false
	}

	return hash, ok
}

func (m *App) getTitleSerieFromCache(tsr *TitleSerieRequest) (*TitleSerie, bool) {
	serie, e := m.cache.PullSerie(tsr.getTitleId(), tsr.getSerieId())
	if e != nil {
		log.Warn().Err(e).Msg("")
		return nil, false
	}

	return serie, serie != nil
}

func (m *App) getTitleSeriesFromApi(titleId string) (_ []*TitleSerie, e error) {
	var title *Title
	e = gAniApi.getApiResponse(http.MethodGet, apiMethodGetTitle,
		[]string{"id", titleId}).parseApiResponse(&title)

	if e != nil {
		return nil, e
	}

	return m.validateTitleFromApiResponse(title), e
}

func (*App) validateTitleFromApiResponse(title *Title) (tss []*TitleSerie) {
	for _, serie := range title.Player.Playlist {
		if serie == nil {
			log.Warn().Msg("there is an empty serie found in the api response's playlist")
			continue
		}

		if serie.Hls == nil {
			log.Warn().Msg("there is an empty serie.HLS found in the api response's playlist")
			continue
		}

		tserie, ok := &TitleSerie{}, false

		tserie.Title = title.Id
		tserie.Serie = serie.Serie
		tserie.QualityHashes = make(map[titleQuality]string)

		if serie.Hls.Sd != "" {
			if tserie.QualityHashes[titleQualitySD], ok = getHashFromUriPath(strings.Split(serie.Hls.Sd, "/")[tsrRawFilename]); !ok {
				log.Warn().Uint16("tid", tserie.Title).Uint16("sed", tserie.Serie).Msg("there is no SD quality for parsed title")
			}
		}

		if serie.Hls.Hd != "" {
			if tserie.QualityHashes[titleQualityHD], ok = getHashFromUriPath(strings.Split(serie.Hls.Hd, "/")[tsrRawFilename]); !ok {
				log.Warn().Uint16("tid", tserie.Title).Uint16("sed", tserie.Serie).Msg("there is no HD quality for parsed title")
			}
		}

		if serie.Hls.Fhd != "" {
			if tserie.QualityHashes[titleQualityFHD], ok = getHashFromUriPath(strings.Split(serie.Hls.Fhd, "/")[tsrRawFilename]); !ok {
				log.Warn().Uint16("tid", tserie.Title).Uint16("sed", tserie.Serie).Msg("there is no FHD quality for parsed title")
			}
		}

		tss = append(tss, tserie)
	}

	return
}

func (m *App) doTitleSerieRequest(tsr *TitleSerieRequest) (ts *TitleSerie, e error) {
	var ok bool

	log.Debug().Str("tid", tsr.getTitleIdString()).Str("sid", tsr.getSerieIdString()).Msg("trying to get series from cache")
	if ts, ok = m.getTitleSerieFromCache(tsr); ok {
		return
	}

	var tss []*TitleSerie
	log.Info().Str("tid", tsr.getTitleIdString()).Str("sid", tsr.getSerieIdString()).Msg("trying to get series from api")
	if tss, e = m.getTitleSeriesFromApi(tsr.getTitleIdString()); e != nil {
		return
	}

	if len(tss) == 0 {
		return nil, errors.New("there is an empty result in the response")
	}

	for _, t := range tss {
		if t.Serie == tsr.getSerieId() {
			ts = t
		}

		if e = m.cache.PushSerie(t); e != nil {
			log.Warn().Err(e).Str("tid", tsr.getTitleIdString()).Str("sid", tsr.getSerieIdString()).Msg("")
			continue
		}
	}

	if ts == nil {
		return nil, errors.New("could not find requesed serie id in the response")
	}

	return
}

type TitleSerieRequest struct {
	titleId, serieId uint16
	quality          titleQuality
	hash             string

	raw []string
}

func NewTitleSerieRequest(uri string) *TitleSerieRequest {
	return &TitleSerieRequest{
		raw: strings.Split(uri, "/"),
	}
}

func (m *TitleSerieRequest) getTitleId() uint16 {
	if m.titleId != 0 {
		return m.titleId
	}

	tid, _ := strconv.ParseUint(m.raw[tsrRawTitleId], 10, 16)
	m.titleId = uint16(tid)
	return m.titleId
}

func (m *TitleSerieRequest) getTitleIdString() string {
	return m.raw[tsrRawTitleId]
}

func (m *TitleSerieRequest) getSerieId() uint16 {
	if m.serieId != 0 {
		return m.serieId
	}

	sid, _ := strconv.ParseUint(m.raw[tsrRawTitleSerie], 10, 16)
	m.serieId = uint16(sid)
	return m.serieId
}

func (m *TitleSerieRequest) getSerieIdString() string {
	return m.raw[tsrRawTitleSerie]
}

func (m *TitleSerieRequest) getTitleQuality() titleQuality {
	if m.quality != titleQualityNone {
		return m.quality
	}

	switch m.raw[tsrRawTitleQuality] {
	case "480":
		m.quality = titleQualitySD
		return m.quality
	case "720":
		m.quality = titleQualityHD
		return m.quality
	case "1080":
		m.quality = titleQualityFHD
		return m.quality
	default:
		return titleQualityNone
	}
}

func (m *TitleSerieRequest) getTitleQualityString() string {
	return m.raw[tsrRawTitleQuality]
}

func (m *TitleSerieRequest) getTitleHash() (_ string, ok bool) {
	if m.hash != "" {
		return m.hash, true
	}

	m.hash, ok = getHashFromUriPath(m.raw[tsrRawFilename])
	return m.hash, ok
}

// TODO - refactor
func (m *TitleSerieRequest) isOldFormat() bool {
	return strings.Contains(m.raw[tsrRawFilename], "fff")
}

func (m *TitleSerieRequest) isM3U8() bool {
	return strings.Contains(m.raw[tsrRawFilename], "m3u8")
}

func (m *TitleSerieRequest) isValid() bool {
	return len(m.raw) == 8
}

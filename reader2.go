package mp4

import (
	"bytes"
	"errors"
	"io"
	"os"

	"github.com/nareix/av"
)

func binSearch(a []mp4index, pos float32) int {
	l := 0
	r := len(a) - 1
	for l < r-1 {
		m := (l + r) / 2
		if pos < a[m].Pos {
			r = m
		} else {
			l = m
		}
		//	l.Printf(" at %d pos %f\n", m, a[m].pos)
	}
	return l
}

func searchIndex(pos float32, trk *mp4trk, key bool) (ret int) {
	if key {
		a := trk.keyFrames
		b := trk.index
		for i := 0; i < len(a)-1; i++ {
			if b[a[i]-1].Pos < pos && pos < b[a[i+1]-1].Pos {
				ret = a[i] - 1
				return
			}
		}
	} else {
		ret = binSearch(trk.index, pos)
	}
	return
}

func testSearchIndex() {
	a := make([]mp4index, 10)
	for i := range a {
		a[i].Pos = float32(i)
	}
	for i := -4; i < 14; i++ {
		pos := float32(i) + 0.1
		//l.Printf("seek: search(%f)", pos)
		binSearch(a, pos)
		//l.Printf("seek: =%d", r)
	}
}

func (m *mp4) SeekKey(pos float32) {
	l.Printf("seek: %f", pos)
	m.vtrk.i = searchIndex(pos, m.vtrk, true)
	l.Printf("seek: V: %f", m.vtrk.index[m.vtrk.i].Pos)
	if m.atrk != nil {
		m.atrk.i = searchIndex(pos, m.atrk, false)
		l.Printf("seek: A: %f", m.atrk.index[m.atrk.i].Pos)
	}
}

func (m *mp4) readTo(trks []*mp4trk, end float32) (ret []*av.Packet, pos float32) {
	for {
		var mt *mp4trk
		for _, t := range trks {
			if t.i >= len(t.index) {
				continue
			}
			if mt == nil || t.index[t.i].Pos < mt.index[mt.i].Pos {
				mt = t
			}
		}
		if mt == nil {
			//l.Printf("mt == nil")
			break
		}
		pos = mt.index[mt.i].Pos
		if pos >= end {
			break
		}
		b := make([]byte, mt.index[mt.i].Size)
		m.rat.ReadAt(b, mt.index[mt.i].Off)
		ret = append(ret, &av.Packet{
			Codec: mt.codec, Key: mt.index[mt.i].Key,
			Pos: mt.index[mt.i].Pos, Data: b,
			//Ts: int64(mt.index[mt.i].ts) * 1000000 / int64(mt.timeScale),
		})
		mt.i++
	}
	return
}

func (m *mp4) GetAAC() []byte {
	return m.AACCfg
}

func (m *mp4) GetPPS() []byte {
	return m.PPS
}

func (m *mp4) GetW() int {
	return m.W
}

func (m *mp4) GetH() int {
	return m.H
}

func (m *mp4) ReadDur(dur float32) (ret []*av.Packet) {
	l.Printf("read: dur %f", dur)
	ret, m.Pos = m.readTo([]*mp4trk{m.vtrk, m.atrk}, m.Pos+dur)
	l.Printf("read: got %d packets", len(ret))
	return
}

func (m *mp4) dumpAtoms(a *mp4atom, indent int) {
	m.logindent = indent
	m.log("%s", a.tag)
	for _, c := range a.childs {
		m.dumpAtoms(c, indent+1)
	}
}

type mp4SourceData interface {
	io.ReadSeeker
	io.ReaderAt
}

func NewMp4(r *bytes.Reader) (m *mp4, err error) {
	m = &mp4{}

	m.rat = mp4SourceData(r)
	m.atom = &mp4atom{}
	m.readAtom(r, 0, nil, m.atom)
	for _, t := range m.Trk {
		m.parseTrk(t)
	}
	if m.vtrk == nil {
		err = errors.New("no video track")
		return
	}
	m.Dur = float32(m.vtrk.dur) / float32(m.vtrk.timeScale)
	return
}

func Open(path string) (m *mp4, err error) {
	m = &mp4{}
	r, err := os.Open(path)
	if err != nil {
		return
	}
	m.rat = r
	m.atom = &mp4atom{}
	m.readAtom(r, 0, nil, m.atom)
	for _, t := range m.Trk {
		m.parseTrk(t)
	}
	if m.vtrk == nil {
		err = errors.New("no video track")
		return
	}
	m.Dur = float32(m.vtrk.dur) / float32(m.vtrk.timeScale)
	return
}

func (m *mp4) wrAtom(w io.Writer, a *mp4atom) {
	if len(a.childs) > 0 {
		if a.tag != "" {
			WriteTag(w, a.tag, func(w io.Writer) {
				for _, ca := range a.childs {
					m.wrAtom(w, ca)
				}
			})
		} else {
			for _, ca := range a.childs {
				m.wrAtom(w, ca)
			}
		}
	} else {
		//  stts: duration array
		//  stsc: defines index count between [chunk#1, chunk#2]
		//  stco: chunkOffs
		//  stsz: sampleSizes
		//  stss: keyFrames
		switch a.tag {
		case "stts":
			m.writeSTTS(w, a.trk.newStts)
		case "stsc":
			m.writeSTSC(w, a.trk.newStsc)
		case "stco":
			m.writeSTCO(w, a.trk.newChunkOffs)
		case "stsz":
			m.writeSTSZ(w, a.trk.newSampleSizes)
		case "stss":
			if len(a.trk.newKeyFrames) > 0 {
				m.writeSTSS(w, a.trk.newKeyFrames)
			}
		case "mdhd":
			m.writeMDHD(w, a.trk)
		case "mvhd":
			m.writeMVHD(w)
		default:
			WriteTag(w, a.tag, func(w io.Writer) {
				w.Write(a.data)
			})
		}
	}
}

func (m *mp4) closeReader() {
	if closer, ok := m.rat.(io.Closer); ok {
		closer.Close()
	}
}

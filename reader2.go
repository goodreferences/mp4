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
		if pos < a[m].pos {
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
			if b[a[i]-1].pos < pos && pos < b[a[i+1]-1].pos {
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
	for i, _ := range a {
		a[i].pos = float32(i)
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
	l.Printf("seek: V: %f", m.vtrk.index[m.vtrk.i].pos)
	if m.atrk != nil {
		m.atrk.i = searchIndex(pos, m.atrk, false)
		l.Printf("seek: A: %f", m.atrk.index[m.atrk.i].pos)
	}
}

func (m *mp4) readTo(trks []*mp4trk, end float32) (ret []*av.Packet, pos float32) {
	for {
		var mt *mp4trk
		for _, t := range trks {
			if t.i >= len(t.index) {
				continue
			}
			if mt == nil || t.index[t.i].pos < mt.index[mt.i].pos {
				mt = t
			}
		}
		if mt == nil {
			//l.Printf("mt == nil")
			break
		}
		pos = mt.index[mt.i].pos
		if pos >= end {
			break
		}
		b := make([]byte, mt.index[mt.i].size)
		m.rat.ReadAt(b, mt.index[mt.i].off)
		ret = append(ret, &av.Packet{
			Codec: mt.codec, Key: mt.index[mt.i].key,
			Pos: mt.index[mt.i].pos, Data: b,
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

func Open(path string) (m *mp4, err error) {
	m = &mp4{}
	r, err := os.Open(path)
	if err != nil {
		return
	}
	m.rat = r
	m.atom = &mp4atom{}
	m.readAtom(r, 0, nil, m.atom)
	for _, t := range m.trk {
		m.parseTrk(t)
	}
	if m.vtrk == nil {
		err = errors.New("no video track")
		return
	}
	m.Dur = float32(m.vtrk.dur) / float32(m.vtrk.timeScale)
	//l.Printf("atoms:")
	//m.dumpAtoms(m.atom, 0)
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

func (m *mp4) DumpTest(outpath string) (err error) {
	// Need modify atoms:
	//  stts: duration array
	//  stsc: defines index count between [chunk#1, chunk#2]
	//  stco: chunkOffs
	//  stsz: sampleSizes
	//  stss: keyFrames

	pos := float32(120)
	minOffStart := int64(2) << 40

	m.vtrk.newIdx = searchIndex(pos, m.vtrk, true)
	newpos := m.vtrk.index[m.vtrk.newIdx].pos
	m.log("adjust pos %v -> %v", pos, newpos)
	pos = newpos

	for _, t := range m.trk {
		m.logindent = 0
		m.log("trk")
		m.logindent = 1

		var ii int
		if t.newIdx != 0 {
			ii = t.newIdx
		} else {
			ii = searchIndex(pos, t, true)
			if ii == 0 {
				ii = searchIndex(pos, t, false)
			}
		}

		t.offStart = t.index[ii].off
		t.newSampleSizes = t.sampleSizes[ii:]
		m.log("ii %d/%d", ii, len(t.sampleSizes))
		m.log("offStart %d", t.offStart)

		m.log("dur %d -> %d", t.dur, t.dur-t.index[ii].ts)
		t.dur -= t.index[ii].ts
		m.Dur = float32(t.dur) / float32(t.timeScale)

		if t.offStart < minOffStart {
			minOffStart = t.offStart
		}

		if len(t.keyFrames) > 0 {
			ik := 0
			for i, s := range t.keyFrames {
				if s >= ii {
					ik = i
					break
				}
			}
			t.newKeyFrames = t.keyFrames[ik:]
			for i := 1; i < len(t.newKeyFrames); i++ {
				t.newKeyFrames[i] -= t.newKeyFrames[0] - 1
			}
			t.newKeyFrames[0] = 1
			m.log("keyFrames %v", t.newKeyFrames[:10])
			m.log("keyFrames %d/%d", ik, len(t.keyFrames))
		}

		iStts := 0
		cnt := 0
		for i, s := range t.stts {
			cnt += s.cnt
			if cnt >= ii {
				break
			}
			iStts = i
		}
		t.newStts = t.stts[iStts:]
		m.log("iStts %d/%d", iStts, len(t.stts))

		//m.log("probestsc %v", t.stsc)
		m.sumStsc(t.stsc, t.chunkOffs)
		iStsc := 0
		iStco := 0
		cnt = 0
		ci := 0
		for ki, _ := range t.chunkOffs {
			for ci+1 < len(t.stsc) && ki+1 == t.stsc[ci+1].first {
				ci++
			}
			cnt += t.stsc[ci].cnt
			//m.log("probe stsc %d #%d ki %d", cnt, ci, ki)
			if cnt >= ii {
				iStsc = ci
				iStco = ki
				cnt -= t.stsc[ci].cnt
				break
			}
		}
		if iStco+1 == t.stsc[iStsc].first {
			t.newStsc = make([]mp4stsc, len(t.stsc)-iStsc)
			copy(t.newStsc, t.stsc[iStsc:])
			t.newStsc[0].cnt -= ii - cnt
		} else {
			t.newStsc = make([]mp4stsc, len(t.stsc)-iStsc+1)
			copy(t.newStsc[1:], t.stsc[iStsc:])
			t.newStsc[0] = t.newStsc[1]
			t.newStsc[0].cnt -= ii - cnt
			t.newStsc[0].first = iStco + 1
			t.newStsc[1].first = iStco + 2
		}
		for i := 0; i < len(t.newStsc); i++ {
			t.newStsc[i].first -= iStco
		}

		t.newChunkOffs = make([]int64, len(t.chunkOffs)-iStco)
		copy(t.newChunkOffs, t.chunkOffs[iStco:])
		t.newChunkOffs[0] = t.offStart
		//m.log("newChunkOffs %v", t.newChunkOffs)
		m.log("iStsc %d/%d", iStsc, len(t.stsc))
		m.log("iStso %d/%d", iStco, len(t.chunkOffs))

		m.log("newStsc %v sum %d", t.newStsc, m.sumStsc(t.newStsc, t.newChunkOffs))
		m.log("newChunkOffs cnt %d", len(t.newChunkOffs))
		m.log("newSampleSizes cnt %d", len(t.newSampleSizes))
	}

	m.logindent = 0
	m.log("gen file")
	var b bytes.Buffer
	m.wrAtom(&b, m.atom)
	newOff := int64(b.Len()) + 8
	m.log("newOff %d minOff %d", newOff, minOffStart)
	st, _ := m.rat.Stat()
	newMdatSize := st.Size() - minOffStart

	m.mdatOff = -minOffStart + newOff
	b.Reset()
	m.wrAtom(&b, m.atom)

	f, _ := os.Create("/tmp/a.mp4")
	f.Write(b.Bytes())
	WriteInt(f, int(newMdatSize)+8, 4)
	WriteString(f, "mdat")
	m.rat.Seek(minOffStart, os.SEEK_SET)
	io.Copy(f, m.rat)
	f.Close()

	return
}

func (m *mp4) closeReader() {
	m.rat.Close()
}

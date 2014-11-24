package mp4

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/nareix/av"
)

func (m *mp4) log(format string, v ...interface{}) {
	l.Printf("parse:%s %s",
		strings.Repeat(" ", m.logindent), fmt.Sprintf(format, v...))
}

func (m *mp4) readFTYP(r io.Reader) {
	s, _ := ReadString(r, 4)
	ver, _ := ReadInt(r, 4)
	bs := ReadAll(r)
	m.log("%v %v %v", s, ver, bs)
}

func (m *mp4) readAVC1(r io.ReadSeeker, indent int, trk *mp4trk) {
	ReadInt(r, 16)
	w, _ := ReadInt(r, 2)
	h, _ := ReadInt(r, 2)
	m.log("- video %dx%d", w, h)
	m.W = w
	m.H = h
	ReadInt(r, 14)
	ReadInt(r, 1)
	ReadBuf(r, 31)
	depth, _ := ReadInt(r, 2)
	tid, _ := ReadInt(r, 2)
	if depth <= 8 {
		panic("unsupported depth < 8")
	}
	m.log("- depth,tid %v %v", depth, tid)
	m.readAtom(r, indent, trk, &mp4atom{})
}

func (m *mp4) readMP4A(r io.ReadSeeker, indent int, trk *mp4trk) {
	ver, _ := ReadInt(r, 2) // version
	if ver != 0 {
		panic("stsd audio ver != 0")
	}
	ReadInt(r, 2)           // reversion
	ReadInt(r, 4)           // vendor
	cr, _ := ReadInt(r, 2)  // channel count
	ReadInt(r, 2)           // sample size
	ReadInt(r, 4)           // cid, packsize
	sr, _ := ReadUint(r, 4) // sample rate
	sr >>= 16
	m.log("- ver %v channels %d samplerate %d", ver, cr, sr)
	m.readAtom(r, indent, trk, &mp4atom{})
}

func readDescrLen(r io.Reader) (ln int) {
	var s [4]int
	for i := 0; i < 4; i++ {
		s[i], _ = ReadInt(r, 1)
	}
	for i := 0; i < 4; i++ {
		c := s[i]
		ln = (ln << 7) | (c & 0x7f)
		if (c & 0x80) == 0 {
			break
		}
	}
	//m.log("descln %x %x %x %x", s[0], s[1], s[2], s[3])
	return
}

func readDescr(r io.Reader) (tag, ln int) {
	tag, _ = ReadInt(r, 1)
	ln = readDescrLen(r)
	return
}

func (m *mp4) readESDS(r io.ReadSeeker, trk *mp4trk) {
	ReadInt(r, 4) // version+flags
	tag, ln1 := readDescr(r)
	m.log("- tag %v %d", tag, ln1)
	if tag == 0x03 {
		ReadInt(r, 2)
		flags, _ := ReadInt(r, 1)
		if (flags & 0x80) != 0 {
			ReadInt(r, 2)
		}
		if (flags & 0x40) != 0 {
			ln, _ := ReadInt(r, 1)
			ReadBuf(r, ln)
		}
		if (flags & 0x20) != 0 {
			ReadInt(r, 2)
		}
	} else {
		ReadInt(r, 2) // id
	}
	tag, _ = readDescr(r)
	if tag == 0x04 {
		objid, _ := ReadInt(r, 1)      // objid
		streamtype, _ := ReadInt(r, 1) // stream type
		ReadInt(r, 3)                  // buffer size db
		rmin, _ := ReadInt(r, 4)       // max bitrate
		rmax, _ := ReadInt(r, 4)       // min bitrate
		m.log("objid %x streamtype %x bitrate %d %d",
			objid, streamtype, rmin, rmax)
		tag2, ln := readDescr(r)
		if tag2 == 0x05 {
			trk.extra, _ = ReadBuf(r, ln)
			m.log("extra %v", ln)
		}
	}
	m.log("- tag %v", tag)
}

func (m *mp4) readSTSD(r io.Reader, indent int, trk *mp4trk) {
	ReadInt(r, 4)
	cnt, _ := ReadInt(r, 4)
	m.log("- entries %d", cnt)
	for i := 0; i < cnt; i++ {
		sz, _ := ReadInt(r, 4)
		cc4, _ := ReadBuf(r, 4)
		if sz >= 16 {
			ReadInt(r, 8)
			sz -= 8
		}
		b, _ := ReadBuf(r, sz-8)
		trk.cc4 = string(cc4)
		m.log("- %s", string(cc4))
		buf := bytes.NewReader(b)
		switch trk.cc4 {
		case "avc1":
			m.readAVC1(buf, indent+1, trk)
		case "mp4a":
			m.readMP4A(buf, indent+1, trk)
		default:
			panic(fmt.Sprintf("unknown stsd %s", trk.cc4))
		}
	}
}

func (m *mp4) readSTTS(r io.Reader, trk *mp4trk) {
	ReadInt(r, 4)
	cnt, _ := ReadInt(r, 4)
	m.log("- cnt %d", cnt)
	trk.stts = make([]mp4stts, cnt)
	for i := 0; i < cnt; i++ {
		trk.stts[i].cnt, _ = ReadInt(r, 4)
		trk.stts[i].dur, _ = ReadInt(r, 4)
	}
}

func (m *mp4) readSTCO(r io.Reader, trk *mp4trk) {
	ReadInt(r, 4)
	cnt, _ := ReadInt(r, 4)
	m.log("- %d", cnt)
	trk.chunkOffs = make([]int64, cnt)
	for i := 0; i < cnt; i++ {
		j, _ := ReadInt(r, 4)
		trk.chunkOffs[i] = int64(j)
	}
}

func (m *mp4) readSTSZ(r io.Reader, trk *mp4trk) {
	ReadInt(r, 4)
	sampsize, _ := ReadInt(r, 4)
	cnt, _ := ReadInt(r, 4)
	m.log("- %d %d", sampsize, cnt)

	trk.sampleSizes = make([]int, cnt)
	sz := (cnt*32 + 4) >> 3
	m.log("- buflen %d", sz)
	for i := 0; i < cnt && sz >= 4; i++ {
		trk.sampleSizes[i], _ = ReadInt(r, 4)
		sz -= 4
	}
	ReadBuf(r, sz)
}

func (m *mp4) readSTSS(r io.Reader, trk *mp4trk) {
	ReadInt(r, 4)
	cnt, _ := ReadInt(r, 4)
	m.log("- keyframes %d", cnt)
	trk.keyFrames = make([]int, cnt)
	for i := 0; i < cnt; i++ {
		trk.keyFrames[i], _ = ReadInt(r, 4)
	}
}

func (m *mp4) readCTTS(r io.Reader) {
	ReadInt(r, 4)
	cnt, _ := ReadInt(r, 4)
	m.log("- %d", cnt)
}

func (m *mp4) readSTSC(r io.Reader, trk *mp4trk) {
	ReadInt(r, 4)
	cnt, _ := ReadInt(r, 4)
	stsc := make([]mp4stsc, cnt)
	for i := 0; i < cnt; i++ {
		stsc[i].first, _ = ReadInt(r, 4)
		stsc[i].cnt, _ = ReadInt(r, 4)
		stsc[i].id, _ = ReadInt(r, 4)
	}
	trk.stsc = stsc
	m.log("- cnt %v", len(stsc))
}

func (m *mp4) readMDHD(r io.Reader, trk *mp4trk) {
	ver, _ := ReadInt(r, 1)
	ReadInt(r, 3)
	if ver == 1 {
		ReadInt(r, 16)
	} else {
		ReadInt(r, 8)
	}
	trk.timeScale, _ = ReadInt(r, 4)
	if ver == 1 {
		trk.dur, _ = ReadInt(r, 8)
	} else {
		trk.dur, _ = ReadInt(r, 4)
	}
	ReadInt(r, 4)
}

func (m *mp4) readAVCC(r io.Reader, trk *mp4trk) {
	trk.extra = ReadAll(r)
	m.log("- %v", trk.extra[:4])
}

func (m *mp4) readAtom(r io.ReadSeeker, indent int, trk *mp4trk, atom *mp4atom) {

	for {
		m.logindent = indent

		size, err := ReadInt(r, 4)
		if err != nil {
			break
		}
		typestr, _ := ReadString(r, 4)
		if size == 1 {
			size2, _ := ReadInt(r, 8)
			size = size2 - 8
		}
		m.log("%s %d", typestr, size)

		curatom := &mp4atom{}
		curatom.tag = typestr
		curatom.trk = trk

		if typestr == "mdat" {
			r.Seek(int64(size-8), 1)
			continue
		}

		b, _ := ReadBuf(r, size-8)
		br := bytes.NewReader(b)

		switch typestr {
		case "ftyp":
			m.readFTYP(br)
		case "moov", "mdia", "minf", "stbl":
			m.readAtom(br, indent+1, trk, curatom)
		case "trak":
			newtrk := &mp4trk{}
			m.Trk = append(m.Trk, newtrk)
			m.readAtom(br, indent+1, newtrk, curatom)
		case "stsd":
			m.readSTSD(br, indent, trk)
		case "stts":
			m.readSTTS(br, trk)
		case "stco":
			m.readSTCO(br, trk)
		case "stsz":
			m.readSTSZ(br, trk)
		case "stss":
			m.readSTSS(br, trk)
		case "ctts":
			m.readCTTS(br)
		case "stsc":
			m.readSTSC(br, trk)
		case "mdhd":
			m.readMDHD(br, trk)
		case "avcC":
			m.readAVCC(br, trk)
		case "esds":
			m.readESDS(br, trk)
		}

		atom.childs = append(atom.childs, curatom)
		if len(curatom.childs) == 0 {
			curatom.data = b
		}
	}
}

func (m *mp4) sumStsc(stsc []mp4stsc, chunkOffs []int64) int {
	cnt := 0
	for i, s := range stsc {
		var j int
		if i == len(stsc)-1 {
			j = len(chunkOffs) - stsc[i].first + 1
		} else {
			j = stsc[i+1].first - stsc[i].first
		}
		//		m.log(" stsc: #%d %d x %d", i, s.cnt, j)
		cnt += s.cnt * j
	}
	return cnt
}

func (m *mp4) parseTrk(trk *mp4trk) {
	m.log("trk %s", trk.cc4)
	m.log(" keyframes cnt %d", len(trk.keyFrames))
	m.log(" chunk cnt %d", len(trk.chunkOffs))
	m.log(" sample cnt %d", len(trk.sampleSizes))
	m.log(" time scale %v", trk.timeScale)
	m.log(" stsc %v", trk.stsc)

	trk.index = make([]mp4index, len(trk.sampleSizes))

	ts := 0
	si := 0
	sj := 0
	ci := 0
	i := 0
	fi := 0

	for ki, off := range trk.chunkOffs {
		for ci+1 < len(trk.stsc) && ki+1 == trk.stsc[ci+1].first {
			ci++
		}
		for j := 0; j < trk.stsc[ci].cnt; j++ {
			size := trk.sampleSizes[i]
			pos := float32(ts) / float32(trk.timeScale)
			trk.index[i] = mp4index{
				Ts: ts, Off: off, Size: size, Pos: pos,
			}
			if false {
				m.log(
					" #%d ts %v off %v size %v chunk off %d #%d/%d,%d/%d stsc %d/%d %d,%d stts %v pos %v %v",
					i, ts, off, size,
					trk.chunkOffs[ki], ki+1, len(trk.chunkOffs), j+1, trk.stsc[ci].cnt,
					ci+1, len(trk.stsc),
					trk.stsc[ci].first, trk.stsc[ci].cnt,
					trk.stts[si], pos, trk.timeScale,
				)
			}

			if fi < len(trk.keyFrames) && i+1 == trk.keyFrames[fi] {
				fi++
				trk.index[i].Key = true
			}

			i++
			off += int64(size)
			ts += trk.stts[si].dur
			sj++
			if sj == trk.stts[si].cnt {
				si++
				sj = 0
			}
		}
	}
	m.log(" si %d len %d", si, len(trk.stts))

	if trk.cc4 == "avc1" {
		trk.codec = av.H264
		m.vtrk = trk
		m.PPS = trk.extra
	}
	if trk.cc4 == "mp4a" {
		trk.codec = av.AAC
		m.atrk = trk
		m.AACCfg = trk.extra
	}
}

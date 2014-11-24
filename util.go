package mp4

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

type logger int

func (l logger) Printf(format string, v ...interface{}) {
	str := fmt.Sprintf(format, v...)
	switch {
	case strings.HasPrefix(str, "parse") && l >= 1,
		strings.HasPrefix(str, "read") && l >= 1:
		l2.Println(str)
	default:
		if l >= 1 {
			l2.Println(str)
		}
	}
}

var (
	l  = logger(0)
	l2 *log.Logger
)

func init() {
	l2 = log.New(os.Stderr, "", 0)
}

type mp4trk struct {
	cc4                         string
	keyFrames, newKeyFrames     []int
	sampleSizes, newSampleSizes []int
	chunkOffs, newChunkOffs     []int64
	stts, newStts               []mp4stts
	stsc, newStsc               []mp4stsc
	index                       []mp4index
	extra                       []byte
	offStart                    int64
	mdatSize                    int
	timeScale                   int
	dur                         int
	i                           int
	newIdx                      int
	codec, idx                  int
}

type mp4stsc struct {
	first, cnt, id int
}

type mp4index struct {
	ts, size int
	off      int64
	key      bool
	pos      float32
}

type mp4stts struct {
	cnt, dur int
}

type mp4atom struct {
	tag    string
	data   []byte
	trk    *mp4trk
	childs []*mp4atom
}

type mp4 struct {
	atom       *mp4atom
	trk        []*mp4trk
	vtrk, atrk *mp4trk
	Dur, Pos   float32
	W, H       int
	rat        *os.File
	AACCfg     []byte
	PPS        []byte
	logindent  int

	durts     int
	timeScale int

	mdatOff   int64
	w, w2     *os.File
	tmp, path string
}

func ReadUint(r io.Reader, n int) (ret uint, err error) {
	b, err := ReadBuf(r, n)
	for i := 0; i < n; i++ {
		ret <<= 8
		ret += uint(b[i])
	}
	return
}

func ReadInt(r io.Reader, n int) (ret int, err error) {
	_ret, err := ReadUint(r, n)
	ret = int(_ret)
	return
}

func WriteInt(r io.Writer, v int, n int) {
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[n-i-1] = byte(v & 0xff)
		v >>= 8
	}
	r.Write(b)
}

func WriteString(w io.Writer, str string) {
	w.Write([]byte(str))
}

func WriteTag(w io.Writer, tag string, cb func(w io.Writer)) {
	var b bytes.Buffer
	cb(&b)
	WriteInt(w, b.Len()+8, 4)
	WriteString(w, tag)
	w.Write(b.Bytes())
}

func ReadAll(r io.Reader) []byte {
	var b bytes.Buffer
	io.Copy(&b, r)
	return b.Bytes()
}

func ReadBuf(r io.Reader, n int) ([]byte, error) {
	b := make([]byte, n)
	n, err := r.Read(b)
	return b, err
}

func ReadString(r io.Reader, n int) (string, error) {
	b, err := ReadBuf(r, n)
	return string(b), err
}

func LogLevel(i int) {
	l = logger(i)
}

func (m *mp4) Close() {
	if m.rat != nil {
		m.closeReader()
	} else {
		m.closeWriter()
	}
}

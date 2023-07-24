package download

import (
	"bytes"
	"io"
	"math/rand"
	"os"
	"strconv"
	"testing"
)

func TestSkipChunks(t *testing.T) {
	c := &Chunks{
		m:           map[int]Chunk{},
		eachChunks:  3,
		isRecovered: true,
	}
	defer c.Remove()
	saveTo, err := os.OpenFile("t.to", os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		panicWithPrefix(err.Error())
	}
	defer saveTo.Close()

	c1 := c.NewChunk(false, "111", 0, 12, 15)
	c1.Write([]byte("197"))
	c2 := c.NewChunk(true, "111", 1, 9, 12)
	c2.Write([]byte("201"))
	c3 := c.NewChunk(true, "111", 2, 6, 9)
	c3.Write([]byte("348"))
	t.Log("c3", c3.Current(), c3.IsDone())
	c4 := c.NewChunk(true, "111", 3, 3, 6)
	c4.Write([]byte("445"))
	c5 := c.NewChunk(true, "111", 4, 0, 3)
	c5.Write([]byte("599"))
	c.CombineOne(c5, saveTo)
	c.CombineOne(c4, saveTo)
	c3.Reset()
	c.CombineLimit(1, c3, saveTo)
	t.Log("c3", c3.Current(), c3.IsDone())
	saveTo.Seek(0, io.SeekStart)
	c.Combine(saveTo, saveTo)
}

func generateChunkContent(id int, n int64) []byte {
	sb := new(bytes.Buffer)
	sb.Grow(int(n))
	sb.WriteString(strconv.Itoa(id))
	for i := 0; i < int(n)-1; i++ {
		sb.WriteString(strconv.Itoa(rand.Intn(10)))
	}
	return sb.Bytes()
}

func generateChunks(eachChunk, n int64, isRecovered ...bool) *Chunks {
	recovered := false
	if len(isRecovered) > 0 {
		recovered = isRecovered[0]
	}
	c := &Chunks{
		m:           map[int]Chunk{},
		eachChunks:  eachChunk,
		isRecovered: recovered,
	}
	size := n * eachChunk
	for i := n - 1; i >= 0; i-- {
		to := size
		size -= eachChunk
		from := size
		c.NewChunk(i > 0, "111", int(i), from, to).Write(generateChunkContent(int(i), to-from))
	}

	return c

}

func TestGetContent(t *testing.T) {
	c := NewChunk("111", 0, 0, 2)
	defer c.Remove()
	c.Write([]byte("22"))
	ret := string(c.Content())
	t.Log(ret)
	if ret != "22" {
		t.Errorf("fail to get the content")
	}

	var b [1]byte
	// should be EOF
	n, err := c.Read(b[:])
	t.Log(n, err)
	if err != io.EOF {
		t.Errorf("fail to reset the pos")
	}
}

func TestSkipMultiChunks(t *testing.T) {
	c := generateChunks(4, 5, true)
	defer c.Remove()
	saveTo, err := os.OpenFile("t.to", os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		panicWithPrefix(err.Error())
	}
	defer saveTo.Close()
	c5 := c.Chunk(5)
	c4 := c.Chunk(4)
	c3 := c.Chunk(3)
	c.CombineOne(c5, saveTo)
	c.CombineLimit(2, c4, saveTo)
	c.CombineLimit(1, c3, saveTo)

	var expectCombined string
	c.Iter(func(i int, c Chunk) bool {
		expectCombined += string(c.Content())
		return true
	})
	c.Combine(saveTo, saveTo)
	saveTo.Seek(0, io.SeekStart)
	b, _ := io.ReadAll(saveTo)
	t.Log(b, expectCombined)
	if string(b) != expectCombined {
		t.Errorf("combine chunks error")
	}
}

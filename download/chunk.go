package download

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

type Chunk interface {
	io.ReadWriteCloser
	// for optimization
	io.ReaderFrom
	EnterWriting()
	ExitWriting()
	Reset()
	CurrentOffset() int64
	OffsetTo(int64)
	Remove()
	Range() string
	ToJSON() string
	Content() []byte
	ID() int
	To() int64
	From() int64
	Current() int64
	Size() int64
	IsDone() bool
	IsCombined() bool
	HasRead() bool
	SetRead(bool)
	SetMinus(bool)
	SetCombined(bool, bool)
}

type chunk struct {
	sync.Mutex
	file      *os.File
	IsRead    bool   `json:"hasRead"`
	Combined  bool   `json:"combined"`
	NeedMinus bool   `json:"needMinus"`
	ChunkID   int    `json:"chunkid"`
	FromBytes int64  `json:"from"`
	ToBytes   int64  `json:"to"`
	Sizes     int64  `json:"size"`
	FileName  string `json:"filename"`
}

func NewChunk(prefix string, id int, from, to int64) Chunk {
	f, err := os.CreateTemp("", "*"+prefix+".chunk")
	if err != nil {
		panicWithPrefix(err.Error())
	}
	return &chunk{
		file:      f,
		ChunkID:   id,
		FromBytes: from,
		ToBytes:   to,
		FileName:  f.Name(),
		Sizes:     to - from,
	}
}
func (c *chunk) EnterWriting() {
	c.Lock()
}
func (c *chunk) ExitWriting() {
	c.Unlock()
}

func (c *chunk) Write(b []byte) (n int, err error) {
	n, err = c.file.Write(b)
	c.FromBytes += int64(n)
	return
}
func (c *chunk) Read(b []byte) (n int, err error) {
	n, err = c.file.Read(b)
	c.FromBytes -= int64(n)
	return
}

// for optimization
func (c *chunk) ReadFrom(r io.Reader) (n int64, err error) {
	n, err = c.file.ReadFrom(r)
	c.FromBytes += n
	return
}

func (c *chunk) Reset() {
	if c.file != nil {
		c.file.Seek(0, io.SeekStart)
	}
}

func (c *chunk) OffsetTo(offset int64) {
	if c.file != nil {
		c.file.Seek(offset, io.SeekStart)
	}
}

func (c *chunk) CurrentOffset() (offset int64) {
	if c.file != nil {
		var err error
		offset, err = c.file.Seek(0, io.SeekCurrent)
		if err != nil {
			return
		}
	}
	return
}

// From() computes the real from value from Size.
func (c *chunk) From() int64 {
	return c.ToBytes - c.Sizes
}

func (c *chunk) Current() int64 {
	return c.FromBytes
}

func (c *chunk) Size() int64 {
	return c.Sizes
}

func (c *chunk) Content() []byte {
	if c.file != nil {
		offset, err := c.file.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil
		}
		// before we started,
		// we need make sure no one is writing.
		c.EnterWriting()
		defer c.ExitWriting()

		c.Reset()
		defer c.OffsetTo(offset)
		b, err := io.ReadAll(c.file)
		if err != nil {
			return nil
		}
		return b
	}
	return nil
}

func (c *chunk) Remove() {
	c.Close()
	os.Remove(c.FileName)
}

func (c *chunk) Close() error {
	// do safe work
	if c.file != nil {
		return c.file.Close()
	}
	return nil
}

func (c *chunk) IsDone() bool {
	return c.FromBytes == c.ToBytes
}

func (c *chunk) IsCombined() bool {
	return c.Combined
}

func (c *chunk) HasRead() bool {
	return c.IsRead
}

func (c *chunk) SetMinus(b bool) {
	c.NeedMinus = b
}
func (c *chunk) SetCombined(b bool, isReset bool) {
	if isReset {
		// if frombytes == to, restore the whole chunks.
		c.FromBytes = c.To()
		c.SetRead(false)
	}
	c.Combined = b
}

func (c *chunk) SetRead(b bool) {
	c.IsRead = b
}

func (c *chunk) Range() string {
	// the reason why to minus one
	// is 0-1000, 1000-2000 causes ranges repeated
	to := c.ToBytes
	if c.NeedMinus {
		to -= 1
	}
	return fmt.Sprintf("bytes=%d-%d", c.FromBytes, to)
}

func (c *chunk) ToJSON() string {
	b, _ := json.Marshal(c)
	return string(b)
}

func (c *chunk) ID() int {
	return c.ChunkID
}
func (c *chunk) To() int64 {
	return c.ToBytes
}

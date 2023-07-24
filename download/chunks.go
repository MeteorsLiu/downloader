package download

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type Chunks struct {
	m           map[int]Chunk
	eachChunks  int64
	isRecovered bool
	allCombined bool
}

type ChunksJson struct {
	ID         string             `json:"id"`
	Options    *DownloaderOptions `json:"options"`
	Chunks     []*chunk           `json:"chunks"`
	EachChunks int64              `json:"eachChunks"`
}

func (c *ChunksJson) Remove() {
	for _, f := range c.Chunks {
		f.Remove()
	}
}

func (c *ChunksJson) RestoreCombine() {
	for _, f := range c.Chunks {
		if f.IsCombined() {
			f.SetCombined(false, true)
		}
	}
}

func NewChunkMap(eachChunks int64) *Chunks {
	return &Chunks{eachChunks: eachChunks, m: make(map[int]Chunk)}
}

func FromJSON(d *Downloader, js []byte) (string, *Chunks) {
	var cj ChunksJson
	json.Unmarshal(js, &cj)
	if len(cj.Chunks) == 0 ||
		cj.EachChunks == 0 ||
		cj.Options.Target == "" {
		printMsg("JSON文件无效")
		return "", nil
	}
	var err error
	c := &Chunks{
		m:           make(map[int]Chunk),
		eachChunks:  cj.EachChunks,
		isRecovered: true,
	}

	for _, ck := range cj.Chunks {
		if !ck.IsCombined() {
			ck.file, err = os.OpenFile(ck.FileName, os.O_RDWR, 0644)
			if err != nil {
				// chunk missed, cannot recover
				// do clean jobs
				cj.Remove()
				printMsgf("区块: %d 恢复失败: %v", ck.ChunkID, err)
				return "", nil
			}
			ck.OffsetTo(ck.Size() - ck.To() + ck.Current())
		}
		c.m[ck.ChunkID] = ck
	}
	if d != nil {
		d.SetOptions(cj.Options)
		d.verbose = Verbose
	}
	return cj.ID, c
}

func (c *Chunks) NewChunk(needMinus bool, prefix string, id int, from, to int64) Chunk {
	chunk := NewChunk(prefix, id, from, to)
	chunk.SetMinus(needMinus)
	c.m[id] = chunk
	return chunk
}

// For special cases, like Test.
func (c *Chunks) CombineOne(chunk Chunk, saveTo io.Writer) {
	if chunk.IsCombined() {
		return
	}
	chunk.Reset()
	chunk.EnterWriting()
	defer chunk.ExitWriting()
	if _, err := io.Copy(saveTo, chunk); err != nil {
		panicWithPrefix("合并失败：" + err.Error())
	}
	chunk.SetCombined(true, false)
}

// For special cases, like Test.
func (c *Chunks) CombineLimit(n int64, chunk Chunk, saveTo io.Writer) {
	if chunk.IsCombined() {
		return
	}
	chunk.Reset()
	chunk.EnterWriting()
	defer chunk.ExitWriting()
	if _, err := io.Copy(saveTo, io.LimitReader(chunk, n)); err != nil {
		panicWithPrefix("合并失败：" + err.Error())
	}
}

// the writer of saveTo may be a *os.File with progress bar.
// we can't do type assertion of it
func (c *Chunks) Combine(saveTo io.Writer, saveToFile *os.File) {
	// start to combine the chunks
	// we need to reverse the chunk map
	// because the calculation of the chunk is reversed
	for id := len(c.m) - 1; id >= 0; id-- {
		f, ok := c.m[id]
		if !ok {
			if c.isRecovered {
				// we can't recover from missed chunks
				c.Remove()
			}
			panicWithPrefixf("区块ID: %d 丢失, 请重试下载", id)
		}
		// skip finished chunks
		if f.IsCombined() {
			saveToFile.Seek(f.To(), io.SeekStart)
			continue
		}

		f.EnterWriting()
		finished := f.To() - f.Current()

		if !f.IsDone() {
			if c.isRecovered {
				saveToFile.Seek(f.From()+finished, io.SeekStart)
			} else {
				panicWithPrefixf("区块ID: %d 未完成, 请重试下载", id)
			}
		}
		// HasRead() makes sure the chunk is completed.
		// and tell Recoverable wether retry to download or not.
		if !f.HasRead() {
			f.SetRead(true)
		}
		f.OffsetTo(finished)
		if _, err := io.Copy(saveTo, f); err != nil {
			fmt.Println(err)
			f.ExitWriting()
			// we got a interrupted signal
			if errors.Is(err, os.ErrClosed) {
				return
			}
			panicWithPrefix("合并失败：" + err.Error())
		}
		f.ExitWriting()
		f.SetCombined(true, false)
	}
	// check if panic or not
	c.allCombined = true
}

func (c *Chunks) Remove() {
	for _, f := range c.m {
		f.Remove()
	}
}

func (c *Chunks) Len() int { return len(c.m) }

func (c *Chunks) Chunk(id int) Chunk {
	if id > 0 {
		return c.m[id-1]
	}
	return c.m[0]
}

func (c *Chunks) RestoreCombine() {
	for _, f := range c.m {
		f.SetCombined(false, true)
	}
}

func (c *Chunks) Close() {
	for _, f := range c.m {
		f.Close()
	}
}

func (c *Chunks) Iter(f func(int, Chunk) bool) {
	for id := len(c.m) - 1; id >= 0; id-- {
		if !f(id, c.m[id]) {
			return
		}
	}
}

func (c *Chunks) Slices() []*chunk {
	chunks := []*chunk{}
	for _, ck := range c.m {
		chunks = append(chunks, ck.(*chunk))
	}
	return chunks
}

func (c *Chunks) ToJSON(opts *DownloaderOptions, oldId ...string) (string, []byte) {
	var id string
	if len(oldId) > 0 {
		id = oldId[0]
	} else {
		id = uuid.NewString()
	}
	cj := &ChunksJson{
		ID:         id,
		Chunks:     c.Slices(),
		EachChunks: c.eachChunks,
		Options:    opts,
	}
	b, _ := json.Marshal(cj)
	return cj.ID, b
}

func (c *Chunks) PreSave() bool {
	tmpDir := getTempPath()
	for _, f := range c.Slices() {
		newName := filepath.Join(tmpDir, filepath.Base(f.FileName))
		if err := os.Rename(f.FileName, newName); err != nil {
			printMsgf("未保存区块迁移失败: %v", err)
			return false
		}
		f.FileName = newName
	}
	return true
}

func (c *Chunks) EnterAllWriting() {
	for _, f := range c.m {
		f.EnterWriting()
	}
}
func (c *Chunks) ExitAllWriting() {
	for _, f := range c.m {
		f.ExitWriting()
	}
}
